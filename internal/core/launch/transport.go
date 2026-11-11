package launch

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/url"
	"slices"
	"strings"
)

const maxClasspathManifest = 1 << 20

// SupportFile returns only non-secret JVM support data. Java 8's JAR contains
// one manifest, no classes, main entry point, agents or executable patches.
// Its absolute file URLs preserve classpath order independently of game cwd.
func (p Plan) SupportFile() (extension string, data []byte, err error) {
	if p.javaMajor != 8 {
		data, err = p.JVMArgumentFile()
		return ".args", data, err
	}
	if p.mainIndex < 1 || p.mainIndex >= len(p.arguments) {
		return "", nil, errors.New("invalid launch plan")
	}
	for _, arg := range p.arguments[:p.mainIndex] {
		for _, secret := range p.secrets {
			if secret != "" && strings.Contains(arg, secret) {
				return "", nil, errors.New("会话资料不能写入 JVM 支持文件")
			}
		}
	}
	index, err := p.classpathIndex()
	if err != nil {
		return "", nil, err
	}
	separator := ":"
	if p.target.OS == "windows" {
		separator = ";"
	}
	data, err = classpathJAR(strings.Split(p.arguments[index], separator), p.target.OS)
	return ".jar", data, err
}

func (p Plan) classpathIndex() (int, error) {
	index := -1
	for i := 0; i < p.mainIndex; i++ {
		if p.arguments[i] != "-cp" && p.arguments[i] != "-classpath" {
			continue
		}
		if index != -1 || i+1 >= p.mainIndex || p.arguments[i+1] == "" {
			return 0, errors.New("启动需要唯一且非空的 classpath 参数")
		}
		index = i + 1
		i++
	}
	if index == -1 {
		return 0, errors.New("启动缺少 classpath 参数")
	}
	return index, nil
}

// ClasspathSupportJAR is shared with short-lived Java8 installation tools. It
// contains only standard local Class-Path URLs, never executable helper code.
func ClasspathSupportJAR(paths []string, targetOS string) ([]byte, error) {
	return classpathJAR(paths, targetOS)
}

func classpathJAR(paths []string, targetOS string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, errors.New("Java 8 classpath 不能为空")
	}
	var value strings.Builder
	for i, path := range paths {
		u, err := classpathURL(path, targetOS)
		if err != nil {
			return nil, err
		}
		if i != 0 {
			value.WriteByte(' ')
		}
		value.WriteString(u)
		if value.Len() > maxClasspathManifest {
			return nil, errors.New("Java 8 classpath 清单超过 1 MiB")
		}
	}
	var manifest bytes.Buffer
	crlf := string([]byte{13, 10})
	manifest.WriteString("Manifest-Version: 1.0" + crlf)
	// Percent-encoded URLs are ASCII; unfolding rejoins split escapes exactly.
	line := "Class-Path: " + value.String()
	for len(line) > 70 {
		manifest.WriteString(line[:70])
		manifest.WriteString(crlf)
		line = " " + line[70:]
	}
	manifest.WriteString(line)
	manifest.WriteString(crlf + crlf)
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: "META-INF/MANIFEST.MF", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := entry.Write(manifest.Bytes()); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return slices.Clone(out.Bytes()), nil
}

func classpathURL(path, targetOS string) (string, error) {
	if !safeText(path) || path == "" || strings.ContainsRune(path, '*') {
		return "", errors.New("Java 8 classpath 路径无效或包含通配符")
	}
	if targetOS == "windows" {
		path = strings.ReplaceAll(path, `\`, "/")
		// UNC and device namespaces require different JVM URL semantics. Do not
		// silently reinterpret them as local drive paths or network resources.
		if strings.HasPrefix(path, "//") || len(path) < 3 || path[1:3] != ":/" ||
			!((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
			return "", errors.New("Java 8 classpath 仅支持本地盘绝对路径，不支持 UNC 或设备路径")
		}
		path = "/" + path
	} else if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", errors.New("Java 8 classpath 必须为本地绝对路径")
	}
	return (&url.URL{Scheme: "file", Path: path}).String(), nil
}
