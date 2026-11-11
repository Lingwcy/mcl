package launch

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"mcl/internal/core/java"
)

type GCMode string

const (
	GCTunedG1         GCMode = "tuned-g1"
	GCG1              GCMode = "g1"
	GCZGC             GCMode = "zgc"
	GCGenerationalZGC GCMode = "generational-zgc"
	GCNone            GCMode = "none"
)

func (mode GCMode) Effective() GCMode {
	if mode == "" {
		return GCTunedG1
	}
	return mode
}

func (mode GCMode) Validate() error {
	switch mode.Effective() {
	case GCTunedG1, GCG1, GCZGC, GCGenerationalZGC, GCNone:
		return nil
	default:
		return errors.New("未知的 JVM 内存管理策略")
	}
}

// CollectorArguments selects flags against the actual probed runtime.
// ZGC is a production collector since 15, generational ZGC arrived in 21
// and became the default ZGC implementation in 23.
func CollectorArguments(mode GCMode, runtime java.Runtime) ([]string, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	mode = mode.Effective()
	if mode == GCGenerationalZGC && runtime.Major < 21 {
		mode = GCZGC
	}
	if (mode == GCZGC || mode == GCGenerationalZGC) && (runtime.Major < 15 || runtime.Arch == "386") {
		mode = GCG1
	}
	switch mode {
	case GCTunedG1:
		return []string{
			"-XX:+UnlockExperimentalVMOptions", "-XX:+UseG1GC",
			"-XX:G1NewSizePercent=20", "-XX:G1ReservePercent=20",
			"-XX:MaxGCPauseMillis=50", "-XX:G1HeapRegionSize=32M",
		}, nil
	case GCG1:
		return []string{"-XX:+UseG1GC"}, nil
	case GCZGC:
		return []string{"-XX:+UseZGC"}, nil
	case GCGenerationalZGC:
		args := []string{"-XX:+UseZGC"}
		if runtime.Major < 23 {
			args = append(args, "-XX:+ZGenerational")
		}
		return args, nil
	default:
		return nil, nil
	}
}

// ParseJVMArguments implements quoting for a text field, never shell expansion.
// Backslashes remain literal except before a matching quote or backslash.
func ParseJVMArguments(text string) ([]string, error) {
	if len(text) > 16<<10 || !utf8.ValidString(text) {
		return nil, errors.New("JVM 参数最多为 16 KiB 的有效文本")
	}
	var args []string
	var word strings.Builder
	var quote rune
	started := false
	runes := []rune(text)
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		if unicode.IsControl(char) {
			return nil, errors.New("JVM 参数不能包含换行或控制字符")
		}
		if char == '\\' && index+1 < len(runes) {
			next := runes[index+1]
			escapedQuote := next == '"' && quote != '\'' || next == '\'' && quote != '"'
			if next == '\\' || escapedQuote {
				index++
				word.WriteRune(next)
				started = true
				continue
			}
		}
		switch {
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
		case char == '"' || char == '\'':
			quote, started = char, true
		case unicode.IsSpace(char):
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
		if len(args) > 128 {
			return nil, errors.New("JVM 参数最多为 128 项")
		}
	}
	if quote != 0 {
		return nil, errors.New("JVM 参数中的引号没有闭合")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) > 128 {
		return nil, errors.New("JVM 参数最多为 128 项")
	}
	return args, nil
}

var jvmOptionName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// ValidateJVMArguments keeps the launch target, heap and native paths owned by
// the launch plan. Collector overrides require the explicit custom mode.
func ValidateJVMArguments(mode GCMode, args []string) error {
	if err := mode.Validate(); err != nil {
		return err
	}
	if len(args) > 128 {
		return errors.New("JVM 参数最多为 128 项")
	}
	total := 0
	for _, arg := range args {
		total += len(arg)
		if total > 16<<10 || !safeText(arg) || arg == "" {
			return errors.New("JVM 参数包含无效内容或超过大小限制")
		}
		if strings.HasPrefix(arg, "-Xmx") || strings.HasPrefix(arg, "-Xms") || strings.HasPrefix(arg, "-XX:MaxHeapSize=") || strings.HasPrefix(arg, "-XX:InitialHeapSize=") ||
			strings.HasPrefix(arg, "-XX:MaxRAM") || strings.HasPrefix(arg, "-XX:MinRAM") || strings.HasPrefix(arg, "-XX:InitialRAM") {
			return errors.New("堆大小由启动器自动管理，JVM 参数不能覆盖内存分配")
		}
		switch {
		case strings.HasPrefix(arg, "-D"):
			key, _, _ := strings.Cut(strings.TrimPrefix(arg, "-D"), "=")
			if key == "" || strings.ContainsAny(key, " \t") || key == "java.class.path" || key == "java.library.path" || key == "sun.java.command" || key == "java.home" || key == "libraryDirectory" || key == "sun.jnu.encoding" || key == "native.encoding" {
				return errors.New("JVM 参数不能覆盖启动路径或主类")
			}
		case strings.HasPrefix(arg, "-XX:"):
			option := strings.TrimPrefix(arg, "-XX:")
			option = strings.TrimLeft(option, "+-")
			name, _, _ := strings.Cut(option, "=")
			if !jvmOptionName.MatchString(name) || name == "Flags" || name == "VMOptionsFile" || name == "OnError" || name == "OnOutOfMemoryError" || name == "SharedArchiveFile" {
				return errors.New("JVM 参数包含不支持的选项文件或外部命令")
			}
			if mode.Effective() != GCNone && (strings.HasPrefix(name, "Use") && strings.HasSuffix(name, "GC") || name == "ZGenerational") {
				return errors.New("自定义垃圾回收器前，请将内存管理选择为「不指定」")
			}
		case strings.HasPrefix(arg, "-Xss"), strings.HasPrefix(arg, "-Xlog:"), strings.HasPrefix(arg, "-verbose:"),
			arg == "-ea", arg == "-da", arg == "-esa", arg == "-dsa":
			// These options do not replace the entry point or inject an agent.
		default:
			return errors.New("不支持的 JVM 参数，允许 -D、-XX、-Xss、-Xlog 和断言选项")
		}
	}
	return nil
}
