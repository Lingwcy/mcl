package loader

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

const maxProcessorClasses = 500_000

// Read headers, never execute bytecode to discover a tool requirement. The
// complete pinned JAR is hashed through this SAME descriptor before and after
// scanning. Future multi-release entries cannot raise the baseline requirement;
// variants selected by supported JVMs must not target a newer class version.
func processorJarMinimum(ctx context.Context, root *os.Root, name string, fixed meta.Download, main string, classes *int, approved ...[]string) (int, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return 0, err
	}
	if !before.Mode().IsRegular() || before.Size() != fixed.Size || fixed.Size <= 0 || fixed.Size > 512<<20 {
		return 0, errors.New("安装工具JAR大小或类型无效")
	}
	file, err := root.Open(name)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return 0, errors.New("安装工具JAR身份已改变")
	}
	verify := func() error {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash := sha1.New()
		n, err := io.Copy(hash, forgeReader{ctx, io.LimitReader(file, fixed.Size+1)})
		if err != nil {
			return err
		}
		if n != fixed.Size || !strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), fixed.SHA1) {
			return errors.New("安装工具JAR固定摘要不符")
		}
		return ctx.Err()
	}
	if err := verify(); err != nil {
		return 0, err
	}
	container, err := archive.Open(file, fixed.Size)
	if err != nil {
		return 0, err
	}
	if container.Has("META-INF/MANIFEST.MF") {
		data, err := container.Read(ctx, "META-INF/MANIFEST.MF", 64<<10)
		if err != nil {
			return 0, err
		}
		var allowed []string
		if len(approved) > 0 {
			allowed = approved[0]
		}
		declared, err := processorManifest(data, name, allowed)
		if err != nil {
			return 0, err
		}
		if main != "" && declared != main {
			return 0, errors.New("已固定工具清单主类与准备结果不符")
		}
	} else if main != "" {
		return 0, errors.New("安装工具JAR缺少已固定清单")
	}
	minimum, mainFound := 8, main == ""
	expectedMain := strings.ReplaceAll(main, ".", "/") + ".class"
	for _, entry := range container.Entries() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if entry.Directory || entry.Name == "module-info.class" || !strings.HasSuffix(entry.Name, ".class") {
			continue
		}
		variant := 0
		if strings.HasPrefix(entry.Name, "META-INF/versions/") {
			parts := strings.Split(entry.Name, "/")
			if len(parts) < 4 {
				return 0, errors.New("安装工具multi-release类路径无效")
			}
			variant, err = strconv.Atoi(parts[2])
			if err != nil || variant < 9 || variant > 100 {
				return 0, errors.New("安装工具multi-release版本无效")
			}
			if variant > 25 {
				continue
			} // Never selected by the supported tool JVM range.
		}
		*classes += 1
		if *classes > maxProcessorClasses {
			return 0, errors.New("安装工具类声明超过50万项")
		}
		header, err := container.ReadPrefix(ctx, entry.Name, 8)
		if err != nil {
			return 0, fmt.Errorf("安装工具class头无效: %w", err)
		}
		if binary.BigEndian.Uint32(header[:4]) != 0xcafebabe {
			return 0, errors.New("安装工具class魔数无效")
		}
		minor, major := binary.BigEndian.Uint16(header[4:6]), binary.BigEndian.Uint16(header[6:8])
		if minor == 65535 || major < 45 || major > 69 {
			return 0, errors.New("安装工具使用预览或超出Java25的class版本")
		}
		required := int(major) - 44
		if variant != 0 {
			if required > variant {
				return 0, errors.New("安装工具multi-release目录与class要求不一致")
			}
		} else {
			minimum = max(minimum, required)
		}
		if entry.Name == expectedMain {
			mainFound = true
		}
	}
	if !mainFound {
		return 0, errors.New("安装工具JAR缺少清单主类的根级class")
	}
	if err := verify(); err != nil {
		return 0, err
	}
	after, err := file.Stat()
	if err != nil {
		return 0, err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return 0, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return 0, errors.New("分析期间安装工具JAR发生变化")
	}
	return minimum, ctx.Err()
}

func (s *ForgeInstaller) processorRuntime(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	if minimum < 8 || minimum > 25 {
		return java.Runtime{}, errors.New("安装工具Java要求超出当前8–25范围")
	}
	var runtime java.Runtime
	var err error
	if s.resolveJava != nil {
		runtime, err = s.resolveJava(ctx, minimum, target, preferred, report)
	} else {
		if len(preferred) == 0 || preferred[0] == "" {
			return java.Runtime{}, errors.New("独立安装工具Java未配置")
		}
		runtime, err = java.Probe(ctx, preferred[0])
	}
	if err != nil {
		return java.Runtime{}, fmt.Errorf("准备安装工具Java（至少%d）: %w", minimum, err)
	}
	if runtime.Major < 8 || runtime.Major > 25 || minimum == 8 && runtime.Major != 8 {
		return java.Runtime{}, errors.New("安装工具Java超出兼容范围；旧class工具固定Java8，不能用高版本替代")
	}
	if err := runtime.Check(minimum, target); err != nil {
		return java.Runtime{}, err
	}
	return runtime, ctx.Err()
}
