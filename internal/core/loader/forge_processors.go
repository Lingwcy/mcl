package loader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
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
	coordinate       string
	position, total  int
	jar              string
	classpath        []string
	args             []string
	outputs          map[string]string
	copyMappings     string
	sanitizeMappings bool
	mappingFormat    string
	nativeTask       string
	inputs           []string
	nativeValues     []forgeValue
	extraction       *processorExtraction
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
	visiting := make(map[string]bool)
	var resolve func(string, int) (forgeValue, error)
	resolve = func(key string, depth int) (forgeValue, error) {
		if value, ok := values[key]; ok {
			return value, nil
		}
		entry, ok := spec.Data[key]
		if !ok || !meta.ValidID(key) || len(entry.Client) > 4096 || depth > 16 || visiting[key] {
			return forgeValue{}, errors.New("安装变量未知、循环或超过边界")
		}
		visiting[key] = true
		defer delete(visiting, key)
		text := entry.Client
		if len(text) > 1 && text[0] == 39 && text[len(text)-1] == 39 {
			text = text[1 : len(text)-1]
		}
		for pos := 0; pos < len(text); {
			start := strings.IndexByte(text[pos:], '{')
			if start < 0 {
				break
			}
			start += pos
			end := strings.IndexByte(text[start+1:], '}')
			if end < 0 {
				return forgeValue{}, errors.New("安装变量未闭合")
			}
			end += start + 1
			if _, err := resolve(text[start+1:end], depth+1); err != nil {
				return forgeValue{}, err
			}
			pos = end + 1
		}
		expanded, err := expandForgeScalars(text, values)
		if err != nil {
			return forgeValue{}, err
		}
		var value forgeValue
		switch {
		case strings.HasPrefix(expanded, "[") && strings.HasSuffix(expanded, "]"):
			relative, err := forgeMaven(expanded[1 : len(expanded)-1])
			if err != nil {
				return value, err
			}
			value = forgeValue{relative, true}
		case strings.HasPrefix(expanded, "/"):
			source := strings.TrimPrefix(expanded, "/")
			if !archive.SafePath(source) {
				return value, errors.New("安装包资源路径无效")
			}
			target := path.Join("work/embedded", source)
			value = forgeValue{target, true}
			if !slices.ContainsFunc(resources, func(m archive.Mapping) bool { return m.Target == target }) {
				resources = append(resources, archive.Mapping{Source: source, Target: target})
			}
		default:
			value, err = forgeArg(expanded, values)
			if err != nil {
				return value, err
			}
		}
		values[key] = value
		return value, nil
	}
	for key := range spec.Data {
		if _, reserved := values[key]; reserved {
			return nil, nil, errors.New("安装清单不能重定义保留变量")
		}
	}
	for key := range spec.Data {
		if _, err := resolve(key, 0); err != nil {
			return nil, nil, err
		}
	}
	return values, resources, nil
}

func forgeArg(input string, values map[string]forgeValue) (forgeValue, error) {
	if len(input) >= 2 && input[0] == 39 && input[len(input)-1] == 39 {
		input = input[1 : len(input)-1]
	}
	var err error
	input, err = expandForgeScalars(input, values)
	if err != nil {
		return forgeValue{}, err
	}
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
		return forgeValue{}, errors.New("未知或不安全的安装处理器变量")
	}
	if !safeForgeLiteral(input) {
		return forgeValue{}, errors.New("不支持的安装处理器参数")
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

func prepareForgeProcessor(spec forgeProcessor, values map[string]forgeValue, stage string) (result preparedProcessor, resultErr error) {
	result.coordinate = spec.Jar
	result.position = spec.Position
	result.total = spec.Total
	defer func() {
		if resultErr != nil {
			resultErr = result.failure("解析声明", resultErr)
		}
	}()
	coordinate, err := meta.ParseCoordinate(spec.Jar)
	if err != nil {
		return result, identityError("工具坐标无效")
	}
	result.coordinate = coordinate.String()
	if !knownForgeProcessor(coordinate) || len(spec.Classpath) > 128 || len(spec.Args) > 128 || len(spec.Outputs) > 16 {
		return result, identityError("工具类型或参数数量尚未支持")
	}
	result.jar = path.Join("libraries", coordinate.Path())
	result.classpath = append(result.classpath, result.jar)
	for _, name := range spec.Classpath {
		p, err := forgeMaven(name)
		if err != nil {
			return result, identityError("classpath坐标无效")
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
	mappingFormat := "TSRG"
	var nativeValues []forgeValue
	sanitize := slices.Contains(spec.Args, "--sanitize")
	for index, input := range spec.Args {
		arg, err := forgeArg(input, values)
		if index > 0 && spec.Args[index-1] == "--from" && coordinate.Artifact == "installertools" {
			source := strings.TrimPrefix(strings.Trim(input, "'"), "/")
			if !archive.SafePath(source) {
				return result, errors.New("归档内部提取路径无效")
			}
			arg, err = forgeValue{source, false}, nil
		}
		if err != nil {
			return result, err
		}
		nativeValues = append(nativeValues, arg)
		if index > 0 {
			switch spec.Args[index-1] {
			case "--task":
				task = arg.text
			case "--version":
				mappingVersion = arg.text
			case "--side":
				mappingSide = arg.text
			case "--format":
				mappingFormat = arg.text
			case "--output", "--out", "--slim", "--extra", "--out-jar", "--outJar", "--outputFile", "--to":
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
			writeFlag := index > 0 && slices.Contains([]string{"--output", "--out", "--slim", "--extra", "--out-jar", "--outJar", "--outputFile", "--to", "--optional"}, spec.Args[index-1])
			if !writeFlag {
				result.inputs = append(result.inputs, arg.text)
			}
			arg.text = filepath.Join(stage, filepath.FromSlash(arg.text))
		}
		result.args = append(result.args, arg.text)
	}
	if coordinate.Artifact == "installertools" {
		switch task {
		case "MCP_DATA", "EXTRACT_FILES", "CREATE_DIR", "CREATE_PARENTS", "BUNDLER_EXTRACT":
			result.nativeTask = task
			result.nativeValues = nativeValues
		case "MERGE_MAPPING", "CHAIN_MAPPING", "EXTRACT_INHERITANCE", "SRG_TO_MCP", "MAPPINGS_CSV":
		case "DOWNLOAD_MOJMAPS":
			if err := validateMappingTask(nativeValues); err != nil {
				return result, err
			}
			if mappingTarget == "" || mappingVersion != values["MINECRAFT_VERSION"].text || mappingSide != "client" {
				return result, errors.New("映射处理器的版本、用途或输出无效")
			}
			result.copyMappings = mappingTarget
			result.sanitizeMappings = sanitize
			result.mappingFormat = mappingFormat
			if sanitize && result.outputs[mappingTarget] == "" {
				return result, errors.New("转换后的映射缺少固定输出摘要")
			}
			if sanitize && mappingFormat != "TSRG" && mappingFormat != "TSRG2" {
				return result, errors.New("映射规范化仅支持TSRG/TSRG2")
			}
		default:
			return result, fmt.Errorf("不支持的客户端安装任务 %q", task)
		}
	}
	if result.nativeTask != "" {
		if err := validateNativeTask(&result); err != nil {
			return result, err
		}
	}
	if len(result.outputs) == 0 && result.nativeTask != "CREATE_DIR" && result.nativeTask != "CREATE_PARENTS" {
		return result, errors.New("安装处理器缺少可核验输出")
	}
	result.inputs = slices.DeleteFunc(result.inputs, func(name string) bool {
		if name == "." || name == "libraries" {
			return true
		}
		if result.nativeTask == "CREATE_DIR" || result.nativeTask == "CREATE_PARENTS" {
			return true
		}
		return false
	})
	return result, nil
}

func forgeOutput(value forgeValue) bool {
	return value.file && strings.HasPrefix(value.text, "libraries/") && archive.SafePath(value.text)
}

func forgeJarMain(ctx context.Context, stage *os.Root, relative string, approved []string) (string, error) {
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
	main, err := processorManifest(data, relative, approved)
	if err != nil {
		return "", err
	}
	if main == "" {
		return "", errors.New("安装处理器缺少有效主类")
	}
	return main, nil
}

// A manifest may only name dependencies already pinned in the explicit
// processor classpath. It cannot add local/remote code or broaden its scope.
func processorManifest(data []byte, relative string, approved []string) (string, error) {
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
		if strings.EqualFold(key, "Class-Path") {
			for _, reference := range strings.Fields(class) {
				u, err := url.Parse(reference)
				if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, string(byte(92))) {
					return "", errors.New("安装工具清单不能使用外部依赖")
				}
				target := path.Clean(path.Join(path.Dir(relative), u.Path))
				if !archive.SafePath(target) || !strings.HasPrefix(target, "libraries/") || !slices.Contains(approved, target) {
					return "", errors.New("清单依赖不在已固定的显式classpath内")
				}
			}
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
