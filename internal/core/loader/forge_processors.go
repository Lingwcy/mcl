package loader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

type forgeValue struct {
	text string
	file bool
}

type preparedProcessor struct {
	jar          string
	classpath    []string
	args         []string
	outputs      map[string]string
	copyMappings string
}

func forgeMaven(value string) (string, error) {
	c, err := meta.ParseCoordinate(value)
	if err != nil {
		return "", err
	}
	return path.Join("libraries", c.Path()), nil
}

func forgeData(spec forgeSpec) (map[string]forgeValue, []archive.Mapping, error) {
	values := map[string]forgeValue{
		"SIDE": {"client", false}, "MINECRAFT_VERSION": {spec.Minecraft, false},
		"ROOT": {".", true}, "LIBRARY_DIR": {"libraries", true},
		"INSTALLER": {"work/installer.jar", true}, "MINECRAFT_JAR": {"work/client.jar", true},
	}
	var resources []archive.Mapping
	for key, side := range spec.Data {
		if _, reserved := values[key]; reserved || !meta.ValidID(key) || len(side.Client) > 4096 {
			return nil, nil, errors.New("安装配置变量无效")
		}
		value := side.Client
		switch {
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			relative, err := forgeMaven(value[1 : len(value)-1])
			if err != nil {
				return nil, nil, err
			}
			values[key] = forgeValue{relative, true}
		case strings.HasPrefix(value, "/data/"):
			source := strings.TrimPrefix(value, "/")
			if !archive.SafePath(source) {
				return nil, nil, errors.New("安装包资源路径无效")
			}
			target := path.Join("work", source)
			values[key] = forgeValue{target, true}
			if !slices.ContainsFunc(resources, func(m archive.Mapping) bool { return m.Target == target }) {
				resources = append(resources, archive.Mapping{Source: source, Target: target})
			}
		default:
			if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) > 1 {
				value = value[1 : len(value)-1]
			}
			values[key] = forgeValue{value, false}
		}
	}
	return values, resources, nil
}

func forgeArg(input string, values map[string]forgeValue) (forgeValue, error) {
	if len(input) > 4096 || strings.ContainsAny(input, "\x00\r\n") {
		return forgeValue{}, errors.New("安装处理器参数无效")
	}
	if strings.HasPrefix(input, "[") && strings.HasSuffix(input, "]") {
		relative, err := forgeMaven(input[1 : len(input)-1])
		return forgeValue{relative, true}, err
	}
	if strings.HasPrefix(input, "{") {
		key, suffix, ok := strings.Cut(input[1:], "}")
		value, found := values[key]
		if ok && found {
			if suffix == "" {
				if value.file || safeForgeLiteral(value.text) {
					return value, nil
				}
			} else if (key == "ROOT" || key == "LIBRARY_DIR") && strings.HasPrefix(suffix, "/") && archive.SafePath(suffix[1:]) {
				return forgeValue{path.Join(value.text, suffix[1:]), true}, nil
			}
		}
		return forgeValue{}, fmt.Errorf("未知或不安全的安装处理器变量 %q", input)
	}
	if !safeForgeLiteral(input) {
		return forgeValue{}, fmt.Errorf("不支持的安装处理器参数 %q", input)
	}
	return forgeValue{input, false}, nil
}

func safeForgeLiteral(input string) bool {
	return !strings.HasPrefix(input, "@") && !strings.ContainsAny(input, "\x00\r\n/\\:{}[]")
}

func knownForgeProcessor(c meta.Coordinate) bool {
	switch c.Group + ":" + c.Artifact {
	case "net.minecraftforge:installertools", "net.minecraftforge:jarsplitter",
		"net.minecraftforge:ForgeAutoRenamingTool", "net.minecraftforge:binarypatcher",
		"net.neoforged.installertools:installertools", "net.neoforged.installertools:jarsplitter",
		"net.neoforged.installertools:binarypatcher", "net.neoforged:AutoRenamingTool",
		"net.neoforged:installertools", "net.neoforged:jarsplitter", "net.neoforged:binarypatcher":
		return c.Extension == "jar"
	}
	return false
}

func prepareForgeProcessor(spec forgeProcessor, values map[string]forgeValue, stage string) (preparedProcessor, error) {
	var result preparedProcessor
	coordinate, err := meta.ParseCoordinate(spec.Jar)
	if err != nil {
		return result, err
	}
	if !knownForgeProcessor(coordinate) || len(spec.Classpath) > 128 || len(spec.Args) > 128 || len(spec.Outputs) > 16 {
		return result, fmt.Errorf("不支持的客户端安装处理器 %s", spec.Jar)
	}
	result.jar = path.Join("libraries", coordinate.Path())
	result.classpath = append(result.classpath, result.jar)
	for _, name := range spec.Classpath {
		p, err := forgeMaven(name)
		if err != nil {
			return result, err
		}
		if !slices.Contains(result.classpath, p) {
			result.classpath = append(result.classpath, p)
		}
	}
	result.outputs = make(map[string]string)
	for target, hash := range spec.Outputs {
		p, err := forgeArg(target, values)
		if err != nil {
			return result, err
		}
		digest, err := forgeArg(hash, values)
		if err != nil {
			return result, err
		}
		if !forgeOutput(p) || digest.file || !meta.ValidSHA1(digest.text) {
			return result, errors.New("安装处理器输出路径或摘要无效")
		}
		if previous, duplicate := result.outputs[p.text]; duplicate && !strings.EqualFold(previous, digest.text) {
			return result, errors.New("安装处理器输出摘要冲突")
		}
		result.outputs[p.text] = digest.text
	}
	var task, mappingTarget, mappingVersion, mappingSide string
	sanitize := slices.Contains(spec.Args, "--sanitize")
	for index, input := range spec.Args {
		arg, err := forgeArg(input, values)
		if err != nil {
			return result, err
		}
		if index > 0 {
			switch spec.Args[index-1] {
			case "--task":
				task = arg.text
			case "--version":
				mappingVersion = arg.text
			case "--side":
				mappingSide = arg.text
			case "--output", "--out", "--slim", "--extra", "--out-jar", "--outJar", "--outputFile":
				if !forgeOutput(arg) {
					return result, errors.New("安装处理器输出必须位于私有库目录")
				}
				if _, known := result.outputs[arg.text]; !known {
					result.outputs[arg.text] = ""
				}
				if spec.Args[index-1] == "--output" {
					mappingTarget = arg.text
				}
			}
		}
		if arg.file {
			arg.text = filepath.Join(stage, filepath.FromSlash(arg.text))
		}
		result.args = append(result.args, arg.text)
	}
	if coordinate.Artifact == "installertools" {
		switch task {
		case "MCP_DATA", "MERGE_MAPPING":
		case "DOWNLOAD_MOJMAPS":
			if mappingTarget == "" || mappingVersion != values["MINECRAFT_VERSION"].text || mappingSide != "client" {
				return result, errors.New("映射处理器的版本、用途或输出无效")
			}
			if !sanitize {
				result.copyMappings = mappingTarget
			} else if result.outputs[mappingTarget] == "" {
				return result, errors.New("转换后的映射缺少上游摘要")
			}
		default:
			return result, fmt.Errorf("不支持的客户端安装任务 %q", task)
		}
	}
	if len(result.outputs) == 0 {
		return result, errors.New("安装处理器缺少可核验输出")
	}
	return result, nil
}

func forgeOutput(value forgeValue) bool {
	return value.file && strings.HasPrefix(value.text, "libraries/") && archive.SafePath(value.text)
}

func forgeJarMain(ctx context.Context, stage *os.Root, relative string) (string, error) {
	jar, err := stage.Open(relative)
	if err != nil {
		return "", err
	}
	defer jar.Close()
	info, err := jar.Stat()
	if err != nil {
		return "", err
	}
	z, err := archive.Open(jar, info.Size())
	if err != nil {
		return "", err
	}
	data, err := z.Read(ctx, "META-INF/MANIFEST.MF", 64<<10)
	if err != nil {
		return "", err
	}
	main, err := processorManifest(data)
	if err != nil {
		return "", err
	}
	if main == "" {
		return "", errors.New("安装处理器缺少有效主类")
	}
	return main, nil
}

// A manifest must not expand the fixed classpath with implicit local/remote
// dependencies. Explicit processor classpaths remain the only source of code.
func processorManifest(data []byte) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var fields []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if suffix, continued := strings.CutPrefix(line, " "); continued && len(fields) > 0 {
			fields[len(fields)-1] += suffix
		} else {
			fields = append(fields, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	main := ""
	for _, line := range fields {
		key, class, found := strings.Cut(line, ": ")
		if !found {
			continue
		}
		if strings.EqualFold(key, "Class-Path") && strings.TrimSpace(class) != "" {
			return "", errors.New("安装工具清单不能隐式扩展固定依赖")
		}
		if !strings.EqualFold(key, "Main-Class") {
			continue
		}
		if main != "" || class == "" || strings.HasPrefix(class, "-") ||
			strings.IndexFunc(class, func(r rune) bool {
				return r != '.' && r != '_' && r != '$' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
			}) != -1 {
			return "", errors.New("安装工具清单主类无效或重复")
		}
		main = class
	}
	return main, nil
}
