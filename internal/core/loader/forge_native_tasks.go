package loader

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

func nativeOptions(p preparedProcessor) (map[string][]forgeValue, error) {
	options := make(map[string][]forgeValue)
	flags := map[string]bool{"--jar-only": true, "--sanitize": true, "--skipIfExists": true}
	for i := 0; i < len(p.nativeValues); i++ {
		option := p.nativeValues[i]
		if option.file || !strings.HasPrefix(option.text, "--") {
			return nil, errors.New("本地处理器选项无效")
		}
		if flags[option.text] {
			options[option.text] = append(options[option.text], forgeValue{"true", false})
			continue
		}
		i++
		if i >= len(p.nativeValues) {
			return nil, errors.New("本地处理器选项缺少值")
		}
		options[option.text] = append(options[option.text], p.nativeValues[i])
	}
	return options, nil
}
func validateMappingTask(values []forgeValue) error {
	options, err := nativeOptions(preparedProcessor{nativeValues: values})
	if err != nil {
		return err
	}
	allowed := []string{"--task", "--version", "--side", "--output", "--sanitize", "--format", "--skipIfExists"}
	for key, entries := range options {
		if !slices.Contains(allowed, key) || len(entries) != 1 {
			return errors.New("官方映射任务选项未知或重复")
		}
	}
	return nil
}

func validateNativeTask(p *preparedProcessor) error {
	options, err := nativeOptions(*p)
	if err != nil {
		return err
	}
	allowed := []string{"--task"}
	required := []string{}
	switch p.nativeTask {
	case "MCP_DATA":
		allowed = append(allowed, "--input", "--output", "--key")
		required = []string{"--input", "--output", "--key"}
	case "EXTRACT_FILES":
		allowed = append(allowed, "--archive", "--from", "--to", "--optional")
		required = []string{"--archive", "--from", "--to"}
	case "CREATE_DIR", "CREATE_PARENTS":
		allowed = append(allowed, "--target")
		required = []string{"--target"}
	case "BUNDLER_EXTRACT":
		allowed = append(allowed, "--input", "--output", "--jar-only")
		required = []string{"--input", "--output", "--jar-only"}
	default:
		return errors.New("未知本地处理器任务")
	}
	for key, values := range options {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("本地客户端任务不支持选项%s", key)
		}
		if key != "--from" && key != "--to" && key != "--optional" && len(values) != 1 {
			return errors.New("本地处理器选项重复")
		}
	}
	for _, key := range required {
		if len(options[key]) == 0 {
			return errors.New("本地处理器缺少必需选项")
		}
	}
	if p.nativeTask == "EXTRACT_FILES" && len(options["--from"]) != len(options["--to"]) {
		return errors.New("提取源和目标数量不一致")
	}
	for _, key := range []string{"--input", "--archive"} {
		for _, value := range options[key] {
			if !value.file || !archive.SafePath(value.text) {
				return errors.New("处理器输入必须是固定的暂存文件")
			}
		}
	}
	for _, key := range []string{"--output", "--to"} {
		for _, value := range options[key] {
			if !forgeOutput(value) {
				return errors.New("本地处理器仅发布私有运行库文件")
			}
			if _, present := p.outputs[value.text]; !present {
				p.outputs[value.text] = ""
			}
		}
	}
	for _, key := range []string{"--input", "--archive"} {
		for _, value := range options[key] {
			for target := range p.outputs {
				if target == value.text || strings.HasPrefix(value.text, target+"/") || strings.HasPrefix(target, value.text+"/") {
					return errors.New("原生归档输入与输出不能重叠")
				}
			}
		}
	}
	for _, value := range options["--target"] {
		if !value.file || !ownedProcessorPath(value.text) {
			return errors.New("目录任务不能离开私有暂存")
		}
	}
	for _, value := range options["--from"] {
		if value.file || !archive.SafePath(strings.TrimPrefix(value.text, "/")) {
			return errors.New("归档内部路径无效")
		}
	}
	for _, value := range options["--optional"] {
		if !value.file || !slices.ContainsFunc(options["--to"], func(to forgeValue) bool { return to == value }) {
			return errors.New("可选目标不在固定提取计划")
		}
	}
	return nil
}
func optionalProcessorOutput(p preparedProcessor, name string) bool {
	if p.nativeTask != "EXTRACT_FILES" {
		return false
	}
	options, err := nativeOptions(p)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(options["--optional"], func(value forgeValue) bool { return value.file && value.text == name })
}

func ownedProcessorPath(value string) bool {
	return value == "." || value == "libraries" || value == "work" || archive.SafePath(value) && (strings.HasPrefix(value, "libraries/") || strings.HasPrefix(value, "work/"))
}

func withForgeArchive(ctx context.Context, stage *os.Root, known map[string]meta.Download, name string, fn func(*archive.Zip) error) error {
	fixed, ok := known[name]
	if !ok || fixed.Size <= 0 || fixed.Size > 512<<20 {
		return errors.New("归档输入没有固定身份")
	}
	before, err := stage.Lstat(name)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return errors.New("归档输入不是普通文件")
	}
	f, err := stage.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) {
		return errors.New("归档输入身份改变")
	}
	verify := func() error {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash := sha1.New()
		n, err := io.Copy(hash, forgeReader{ctx, io.LimitReader(f, fixed.Size+1)})
		if err != nil {
			return err
		}
		if n != fixed.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), fixed.SHA1) {
			return errors.New("归档输入摘要改变")
		}
		return ctx.Err()
	}
	if err := verify(); err != nil {
		return err
	}
	z, err := archive.Open(f, fixed.Size)
	if err != nil {
		return err
	}
	if err := fn(z); err != nil {
		return err
	}
	if err := verify(); err != nil {
		return err
	}
	after, err := stage.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return errors.New("处理期间归档输入被替换")
	}
	return ctx.Err()
}
func runNativeProcessor(ctx context.Context, stage *os.Root, known map[string]meta.Download, p preparedProcessor) error {
	options, err := nativeOptions(p)
	if err != nil {
		return err
	}
	first := func(key string) string { return options[key][0].text }
	switch p.nativeTask {
	case "CREATE_DIR", "CREATE_PARENTS":
		target := first("--target")
		if p.nativeTask == "CREATE_PARENTS" {
			target = path.Dir(target)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return stage.MkdirAll(target, 0o700)
	case "EXTRACT_FILES":
		return withForgeArchive(ctx, stage, known, first("--archive"), func(z *archive.Zip) error {
			var mappings []archive.Mapping
			for index, from := range options["--from"] {
				target := options["--to"][index].text
				if optionalProcessorOutput(p, target) {
					if _, exists := known[target]; exists {
						continue
					}
				}
				mappings = append(mappings, archive.Mapping{Source: strings.TrimPrefix(from.text, "/"), Target: options["--to"][index].text})
			}
			return z.Extract(ctx, stage, mappings)
		})
	case "MCP_DATA":
		return withForgeArchive(ctx, stage, known, first("--input"), func(z *archive.Zip) error {
			data, err := z.Read(ctx, "config.json", 4<<20)
			if err != nil {
				return err
			}
			var config struct{ Data map[string]json.RawMessage }
			if err := json.Unmarshal(data, &config); err != nil {
				return err
			}
			keys := strings.Split(first("--key"), ".")
			if len(keys) > 16 {
				return errors.New("MCP数据键超过边界")
			}
			values := config.Data
			for index, key := range keys {
				raw, ok := values[key]
				if !ok {
					return errors.New("MCP数据键不存在")
				}
				if index == len(keys)-1 {
					var source string
					if err := json.Unmarshal(raw, &source); err != nil {
						return err
					}
					if strings.HasSuffix(source, "/") {
						prefix := strings.TrimSuffix(source, "/")
						if !archive.SafePath(prefix) || p.outputs[first("--output")] != "" {
							return errors.New("MCP目录声明或摘要无效")
						}
						var mappings []archive.Mapping
						for _, entry := range z.Entries() {
							if entry.Directory || !strings.HasPrefix(entry.Name, source) {
								continue
							}
							suffix := strings.TrimPrefix(entry.Name, source)
							mappings = append(mappings, archive.Mapping{Source: entry.Name, Target: path.Join(first("--output"), suffix)})
							if len(mappings) > 4096 {
								return errors.New("MCP目录输出超过4096项")
							}
						}
						if len(mappings) == 0 {
							return errors.New("MCP目录数据为空")
						}
						if err := z.Extract(ctx, stage, mappings); err != nil {
							return err
						}
						delete(p.outputs, first("--output"))
						for _, mapping := range mappings {
							p.outputs[mapping.Target] = ""
						}
						return nil
					}
					if !archive.SafePath(source) {
						return errors.New("MCP客户端数据路径无效")
					}
					return z.Extract(ctx, stage, []archive.Mapping{{Source: source, Target: first("--output")}})
				}
				if err := json.Unmarshal(raw, &values); err != nil {
					return err
				}
			}
			return errors.New("MCP数据键为空")
		})
	case "BUNDLER_EXTRACT":
		return withForgeArchive(ctx, stage, known, first("--input"), func(z *archive.Zip) error {
			manifest, err := z.Read(ctx, "META-INF/MANIFEST.MF", 64<<10)
			if err != nil {
				return err
			}
			normalized := strings.ReplaceAll(string(manifest), string([]byte{13, 10}), string([]byte{10}))
			if !slices.Contains(strings.Split(normalized, string([]byte{10})), "Bundler-Format: 1.0") {
				return errors.New("仅支持Bundler1.0")
			}
			list, err := z.Read(ctx, "META-INF/versions.list", 1<<20)
			if err != nil {
				return err
			}
			lines := strings.Split(strings.TrimSpace(string(list)), string([]byte{10}))
			if len(lines) != 1 {
				return errors.New("Bundler客户端主JAR必须唯一")
			}
			fields := strings.Split(strings.TrimSpace(lines[0]), string([]byte{9}))
			if len(fields) != 3 || len(fields[0]) != 64 || !archive.SafePath(fields[2]) || !strings.HasSuffix(fields[2], ".jar") {
				return errors.New("Bundler主JAR记录无效")
			}
			expected, err := hex.DecodeString(fields[0])
			if err != nil {
				return err
			}
			source := "META-INF/versions/" + fields[2]
			if err := z.Extract(ctx, stage, []archive.Mapping{{Source: source, Target: first("--output")}}); err != nil {
				return err
			}
			output, err := stage.Open(first("--output"))
			if err != nil {
				return err
			}
			defer output.Close()
			hash := sha256.New()
			if _, err := io.Copy(hash, forgeReader{ctx, io.LimitReader(output, (512<<20)+1)}); err != nil {
				return err
			}
			if !slices.Equal(hash.Sum(nil), expected) {
				return errors.New("Bundler主JAR SHA256不符")
			}
			return ctx.Err()
		})
	}
	return errors.New("未知本地客户端处理器")
}
