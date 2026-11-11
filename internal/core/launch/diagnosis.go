package launch

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const MaxDiagnosticFindings = 12

type RuntimeObservation struct{ Code, Value string }
type RuntimeFinding struct {
	Code, Title, Next, Confidence string
	Evidence                      []string
}
type RuntimeDiagnosis struct {
	Observations []RuntimeObservation
	Findings     []RuntimeFinding
	Sources      []DiagnosticLog
}
type DiagnosticLog struct {
	Kind, Association string
	Lines             []string
	Truncated         bool
}

var diagnosticURL = regexp.MustCompile(`https?://[^\x00-\x1f <>"']+`)
var diagnosticCredential = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|client[_-]?secret|api[_-]?key|id[_-]?token|password|cookie)(["']? *[:=] *["']?)([^ \t,"';]+)`)
var diagnosticJWT = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
var diagnosticAuthorization = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|cookie|set-cookie|access[_-]?token|refresh[_-]?token|client[_-]?secret|api[_-]?key|id[_-]?token|password)["']?\s*[:=]\s*[^\r\n]+`)

// DiagnosticText is used only for bounded excerpts, never for copying complete
// launch commands. Known session secrets are scrubbed BEFORE generic filters.
func DiagnosticText(value string, redact func(string) string) string {
	if len(value) > 64<<10 {
		return "[过长诊断行已省略]"
	}
	if redact != nil {
		value = redact(value)
	}
	lower := strings.ToLower(value)
	for _, label := range []string{"command line:", "java_command:", "jvm_args:", "launch arguments:", "launching with arguments", "--accesstoken", "--session ", "--username "} {
		if strings.Contains(lower, label) {
			return "[启动命令已省略]"
		}
	}
	value = diagnosticAuthorization.ReplaceAllString(value, "$1: [REDACTED]")
	value = diagnosticURL.ReplaceAllString(value, "[URL]")
	value = diagnosticCredential.ReplaceAllString(value, "$1$2[REDACTED]")
	value = diagnosticJWT.ReplaceAllString(value, "[REDACTED]")
	value = strings.ToValidUTF8(value, "�")
	if utf8.RuneCountInString(value) > 240 {
		value = string([]rune(value)[:240]) + "…"
	}
	return value
}

func DiagnoseRuntime(output OutputSnapshot, exit Exit, running bool, window string, sources []DiagnosticLog) RuntimeDiagnosis {
	switch window {
	case "pending", "visible", "unavailable", "unsupported":
	default:
		window = "unknown"
	}
	result := RuntimeDiagnosis{Observations: []RuntimeObservation{{"process", map[bool]string{true: "running", false: "exited"}[running]}, {"window", window}, {"main-menu", "unconfirmed"}}}
	if window == "visible" {
		result.Observations = append(result.Observations, RuntimeObservation{"window-type", "unclassified-process-window"})
	}
	if !running {
		result.Observations = append(result.Observations, RuntimeObservation{"exit-code", fmt.Sprint(exit.Code)})
		if exit.Stopped {
			result.Observations = append(result.Observations, RuntimeObservation{"exit-owner", "user-stop"})
		} else if exit.Detached {
			result.Observations = append(result.Observations, RuntimeObservation{"exit-owner", "detached"})
		}
	}
	if output.Dropped > 0 || exit.OutputIncomplete {
		result.Observations = append(result.Observations, RuntimeObservation{"output", "incomplete"})
	}
	positions := make(map[string]int)
	observations := make(map[string]bool)
	addObservation := func(code string) {
		if !observations[code] {
			observations[code] = true
			result.Observations = append(result.Observations, RuntimeObservation{code, "log-observed"})
		}
	}
	add := func(code, title, next, line string) {
		i, found := positions[code]
		if !found {
			if len(result.Findings) >= MaxDiagnosticFindings {
				return
			}
			i = len(result.Findings)
			positions[code] = i
			result.Findings = append(result.Findings, RuntimeFinding{Code: code, Title: title, Next: next, Confidence: "observed-signal"})
		}
		evidence := DiagnosticText(line, nil)
		if len(result.Findings[i].Evidence) < 2 && !slices.Contains(result.Findings[i].Evidence, evidence) {
			result.Findings[i].Evidence = append(result.Findings[i].Evidence, evidence)
		}
	}
	inspect := func(text string) {
		lower := strings.ToLower(text)
		has := func(terms ...string) bool {
			for _, term := range terms {
				if strings.Contains(lower, term) {
					return true
				}
			}
			return false
		}
		if has("modlauncher running", "forge mod loader version", "loading tweak class name", "fabric loader", "quilt loader") {
			addObservation("loader-initialization")
		}
		if has("lwjgl version", "backend library: lwjgl", "opengl version") {
			addObservation("graphics-initialization")
		}
		if has("openal initialized", "sound engine started", "sound engine is running") {
			addObservation("audio-initialization")
		}
		switch {
		case has("unsupportedclassversionerror", "has been compiled by a more recent version of the java runtime"):
			add("java-class-version", "Java 与类文件要求不匹配", "核对本次游戏与工具 Java 的固定需求及实际版本。", text)
		case has("inaccessibleobjectexception", "illegalaccesserror"):
			add("java-access", "Java 模块或访问限制异常", "核对加载器与当前 Java 的官方兼容要求，不自动添加绕过参数。", text)
		case has("noclassdeffounderror", "classnotfoundexception"):
			add("class-loading", "类加载失败", "核对相关类来自游戏库、加载器还是模组，再按固定安装计划检查文件与版本。", text)
		case has("unsatisfiedlinkerror", "wrong elf class", "not a valid win32 application", "incompatible architecture", "no suitable image found"):
			add("native-load", "本地库加载失败", "核对 Java、目标平台和本地库架构；文件头不代表动态依赖已满足。", text)
		case has("glfw error 65542", "failed to create window", "could not create opengl context", "pixel format not accelerated"):
			add("graphics-window", "图形窗口初始化失败", "核对图形驱动、显示会话与本地库；不自动更换图形库或修改系统权限。", text)
		case has("missing mandatory dependencies", "modresolutionexception", "incompatible mod set", "missing or unsupported mandatory dependencies"):
			add("mod-dependency", "加载器报告模组依赖问题", "按报告中的模组和版本检查依赖，不自动删除整合包文件。", text)
		case has("mixinapplyerror", "mixintransformererror", "invalidmixinexception"):
			add("mod-transformation", "模组转换失败", "查看关联模组与转换错误；单条异常不能证明需要升级所有库。", text)
		case has("outofmemoryerror"):
			add("memory", "运行时内存分配失败", "区分 Java 堆、本地内存或系统资源不足，结合本次自动分配和系统可用量检查。", text)
		case has("invalid session", "authenticationexception", "failed to verify username"):
			add("authentication", "会话或服务器认证失败", "检查账户授权与服务器连接；不导出令牌或完整启动参数。", text)
		case has("unknownhostexception", "sockettimeoutexception", "sslhandshakeexception", "connectexception"):
			add("external-network", "外部网络异常", "确认异常来自游戏服务、模组服务还是服务器；启动器下载代理不代表游戏代理。", text)
		case has("a fatal error has been detected by the java runtime", "exception_access_violation", "sigsegv"):
			add("jvm-native-crash", "JVM 报告原生崩溃", "核对本次 hs_err 的线程、模块与架构证据；不直接把最后一条警告当根因。", text)
		}
	}
	for _, line := range output.Lines[max(0, len(output.Lines)-MaxLogLines):] {
		inspect(DiagnosticText(line.Text, nil))
	}
	for _, source := range sources[:min(4, len(sources))] {
		switch source.Kind {
		case "latest", "debug", "crash-report", "jvm-error":
		default:
			continue
		}
		copy := source
		switch copy.Association {
		case "changed-in-session", "pid-and-session":
		default:
			copy.Association = "unconfirmed"
		}
		copy.Lines = nil
		for _, line := range source.Lines[max(0, len(source.Lines)-256):] {
			copy.Lines = append(copy.Lines, DiagnosticText(line, nil))
		}
		result.Sources = append(result.Sources, copy)
		for _, line := range copy.Lines {
			inspect(line)
		}
	}
	if len(result.Findings) == 0 {
		next := "目前没有足够异常证据；结合本次会话日志和窗口观测继续定位。"
		if running && window == "pending" {
			next = "进程仍在运行但尚未观察到可见窗口。查看本次日志；声音初始化和运行时长不能证明主菜单已出现。"
		}
		result.Findings = []RuntimeFinding{{Code: "insufficient-evidence", Title: "证据不足", Next: next, Confidence: "insufficient"}}
	}
	return result
}

func (r RuntimeDiagnosis) Text() string {
	var out strings.Builder
	for _, o := range r.Observations {
		fmt.Fprintf(&out, "%s=%s\n", o.Code, o.Value)
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&out, "\n[%s] %s confidence=%s cause=unconfirmed\n", f.Code, f.Title, f.Confidence)
		for _, line := range f.Evidence {
			fmt.Fprintln(&out, "  "+line)
		}
		fmt.Fprintln(&out, f.Next)
	}
	for _, source := range r.Sources {
		fmt.Fprintf(&out, "\nsource=%s association=%s truncated=%t\n", source.Kind, source.Association, source.Truncated)
		for _, line := range source.Lines {
			fmt.Fprintln(&out, line)
		}
	}
	return out.String()
}
