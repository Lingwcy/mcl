package java

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mcl/internal/fileio"
)

type Progress struct {
	Stage        string
	Bytes, Total int64
}

// Installer publishes a verified private runtime without changing system Java.
type Installer struct{ Directory string }

func (i *Installer) Install(ctx context.Context, p Package, report func(Progress)) (Runtime, error) {
	if err := p.Validate(); err != nil {
		return Runtime{}, err
	}
	if err := os.MkdirAll(i.Directory, 0o700); err != nil {
		return Runtime{}, err
	}
	root, err := os.OpenRoot(i.Directory)
	if err != nil {
		return Runtime{}, err
	}
	defer root.Close()
	name := "temurin-" + p.ID()
	inspect := func(directory string) (Runtime, error) {
		executable, err := installedExecutable(ctx, directory)
		if err != nil {
			return Runtime{}, err
		}
		r, err := Probe(ctx, executable)
		if err != nil {
			return Runtime{}, err
		}
		if r.Major != p.Major || r.Arch != p.Arch || r.EarlyAccess {
			return Runtime{}, errors.New("下载的 Java 与所选版本或架构不一致")
		}
		return r, nil
	}
	if info, err := root.Lstat(name); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Runtime{}, errors.New("Java 安装目标已存在且不是普通目录")
		}
		return inspect(filepath.Join(i.Directory, name))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Runtime{}, err
	}
	stage, err := os.MkdirTemp(i.Directory, ".install-")
	if err != nil {
		return Runtime{}, err
	}
	defer os.RemoveAll(stage)
	archive, err := os.CreateTemp(i.Directory, ".download-")
	if err != nil {
		return Runtime{}, err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	task, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if err := downloadPackage(task, archive, p, report); err != nil {
		return Runtime{}, err
	}
	if report != nil {
		report(Progress{Stage: "解压并检查 Java", Bytes: p.Size, Total: p.Size})
	}
	if err := extractPackage(task, archive, stage, p.OS == "windows"); err != nil {
		return Runtime{}, err
	}
	runtime, err := inspect(stage)
	if err != nil {
		return Runtime{}, err
	}
	executable, err := filepath.Rel(stage, runtime.Executable)
	if err != nil || !filepath.IsLocal(executable) {
		return Runtime{}, errors.New("Java 可执行文件超出安装目录")
	}
	home, err := filepath.Rel(stage, runtime.Home())
	if err != nil || !filepath.IsLocal(home) {
		return Runtime{}, errors.New("Java Home 超出安装目录")
	}
	if err := task.Err(); err != nil {
		return Runtime{}, err
	}
	if err := fileio.PublishDirectory(root, filepath.Base(stage), name); err != nil {
		return Runtime{}, fmt.Errorf("发布 Java 环境: %w", err)
	}
	// Publication is the commit point. Cancellation after it keeps the complete
	// runtime discoverable on the next scan.
	runtime.Executable = filepath.Join(i.Directory, name, executable)
	runtime.HomeDirectory = filepath.Join(i.Directory, name, home)
	return runtime, nil
}

func downloadPackage(ctx context.Context, file *os.File, p Package, report func(Progress)) error {
	client := &http.Client{
		Timeout: 15 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 || !officialJavaDownload(req.URL) {
				return errors.New("Java 下载重定向不是官方发行地址")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mcl/0.1 (Meow Craft Launcher)")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载 Java: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Java 下载返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > 0 && response.ContentLength != p.Size {
		return errors.New("Java 下载大小与官方目录不一致")
	}
	hash := sha256.New()
	writer := &javaDownloadWriter{ctx: ctx, destination: io.MultiWriter(file, hash), total: p.Size, report: report}
	if report != nil {
		report(Progress{Stage: "下载 Java", Total: p.Size})
	}
	n, err := io.Copy(writer, io.LimitReader(response.Body, p.Size+1))
	if err != nil {
		return err
	}
	if n != p.Size || hex.EncodeToString(hash.Sum(nil)) != p.SHA256 {
		return errors.New("Java 安装包大小或 SHA-256 校验失败")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}

func officialJavaDownload(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Host {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

type javaDownloadWriter struct {
	ctx         context.Context
	destination io.Writer
	written     int64
	total       int64
	report      func(Progress)
	last        time.Time
}

func (w *javaDownloadWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.written+int64(len(data)) > w.total {
		return 0, errors.New("Java 下载超过声明大小")
	}
	n, err := w.destination.Write(data)
	w.written += int64(n)
	if w.report != nil && time.Since(w.last) >= 100*time.Millisecond {
		w.last = time.Now()
		w.report(Progress{Stage: "下载 Java", Bytes: w.written, Total: w.total})
	}
	return n, err
}

func installedExecutable(ctx context.Context, directory string) (string, error) {
	var found string
	depth := 100
	err := filepath.WalkDir(directory, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.Count(filepath.ToSlash(relative), "/") > 5 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && (entry.Name() == "java" || entry.Name() == "java.exe") && filepath.Base(filepath.Dir(name)) == "bin" {
			currentDepth := strings.Count(filepath.ToSlash(relative), "/")
			// Java 8 JDKs also contain a nested jre/bin/java.
			if currentDepth == depth {
				return errors.New("Java 安装包包含多个运行环境")
			}
			if currentDepth < depth {
				found, depth = name, currentDepth
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("Java 安装包缺少可执行文件")
	}
	return found, nil
}

// Home returns the home associated with the inspected executable.
func (r Runtime) Home() string {
	if filepath.IsAbs(r.HomeDirectory) {
		return r.HomeDirectory
	}
	return filepath.Dir(filepath.Dir(r.Executable))
}

// ManagedCandidates returns only completed runtime directories.
func ManagedCandidates(ctx context.Context, directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "temurin-") {
			if executable, err := installedExecutable(ctx, filepath.Join(directory, entry.Name())); err == nil {
				paths = append(paths, executable)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func safeLink(name, target string) bool {
	if target == "" || path.IsAbs(target) || strings.ContainsAny(target, "\\:\x00") {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved != "." && resolved != ".." && !strings.HasPrefix(resolved, "../") &&
		strings.Split(name, "/")[0] == strings.Split(resolved, "/")[0]
}
