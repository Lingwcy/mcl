// Package launch builds vanilla JVM commands without executing them.
package launch

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

// Session is an ephemeral Minecraft identity. The account service validates
// online sessions before handing them to launch. Its token never enters JSON.
type Session struct {
	Name, UUID, ClientID, XUID string
	ExpiresAt                  time.Time
	token                      string
	offline                    bool
}

func NewSession(name, uuid, token, clientID, xuid string, expires time.Time) (Session, error) {
	s := Session{Name: name, UUID: uuid, ClientID: clientID, XUID: xuid, ExpiresAt: expires, token: token}
	if err := s.validate(time.Now()); err != nil {
		return Session{}, err
	}
	return s, nil
}

func NewOfflineSession(name, uuid string) (Session, error) {
	s := Session{Name: name, UUID: uuid, offline: true}
	if err := s.validate(time.Now()); err != nil {
		return Session{}, err
	}
	return s, nil
}

func (s Session) validate(now time.Time) error {
	_, err := hex.DecodeString(s.UUID)
	if len(s.UUID) != 32 || err != nil || strings.TrimSpace(s.Name) == "" || len(s.Name) > 64 {
		return errors.New("Minecraft 玩家资料无效")
	}
	if !s.offline && (strings.TrimSpace(s.ClientID) == "" || len(s.ClientID) > 128 || len(s.XUID) > 64 || len(s.token) < 1 || len(s.token) > 16<<10 || !s.ExpiresAt.After(now.Add(30*time.Second))) {
		return errors.New("Minecraft 会话资料缺失或即将过期，请重新登录")
	}
	for _, value := range []string{s.Name, s.UUID, s.ClientID, s.XUID, s.token} {
		if !safeText(value) {
			return errors.New("Minecraft 会话包含无效字符")
		}
	}
	return nil
}

func (s Session) String() string {
	return fmt.Sprintf("MinecraftSession{name:%q, token:[REDACTED]}", s.Name)
}
func (s Session) GoString() string { return s.String() }
func (s Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name, UUID string
		ExpiresAt  time.Time
	}{s.Name, s.UUID, s.ExpiresAt})
}

type Options struct {
	Root                  string
	GameDirectory         string
	InstanceID            string
	Version               meta.Version
	Platform              meta.Platform
	Java                  java.Runtime
	JavaRequirement       java.Requirement
	LegacyAssetsDirectory string
	NativeDirectory       string
	MaxMemoryMiB          int
	MinMemoryMiB          int
	GCMode                GCMode
	JVMArguments          []string
	Game                  GameSettings
}

// Plan keeps executable arguments private so common formatting and JSON
// diagnostics cannot accidentally disclose the session token.
type Plan struct {
	priority                                ProcessPriority
	executable, directory, root, instanceID string
	arguments, secrets                      []string
	mainIndex, javaMajor                    int
	nativeEncoding                          string
	target                                  meta.Platform
}

type Preview struct {
	mainIndex  int
	Priority   ProcessPriority `json:"process_priority,omitempty"`
	Executable string          `json:"executable"`
	Directory  string          `json:"directory"`
	Arguments  []string        `json:"arguments"`
}

func Build(options Options, session Session) (Plan, error) {
	if err := session.validate(time.Now()); err != nil {
		return Plan{}, err
	}
	preview, err := build(options, &session)
	if err != nil {
		return Plan{}, err
	}
	index := preview.mainIndex
	return Plan{priority: options.Game.Priority, executable: preview.Executable, directory: preview.Directory, root: options.Root, instanceID: options.InstanceID, arguments: preview.Arguments, mainIndex: index, javaMajor: options.Java.Major, nativeEncoding: options.Java.NativeEncoding, secrets: []string{session.token}, target: meta.Platform{OS: options.Platform.OS, Arch: meta.NormalizeArch(options.Platform.Arch)}}, nil
}

// PreviewOptions produces a reviewable command with explicit unresolved login
// placeholders. It does not construct a session or an executable launch Plan.
func PreviewOptions(options Options) (Preview, error) { return build(options, nil) }

func (p Plan) Preview() Preview {
	args := slices.Clone(p.arguments)
	for i, arg := range args {
		args[i] = p.Redact(arg)
	}
	return Preview{Priority: p.priority, Executable: p.Redact(p.executable), Directory: p.Redact(p.directory), Arguments: args}
}
func (p Plan) Redact(text string) string {
	for _, secret := range p.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}
func (p Plan) String() string               { data, _ := p.MarshalJSON(); return string(data) }
func (p Plan) GoString() string             { return p.String() }
func (p Plan) MarshalJSON() ([]byte, error) { return json.Marshal(p.Preview()) }

func (p Plan) Executable() string        { return p.executable }
func (p Plan) Priority() ProcessPriority { return p.priority }
func (p Plan) Directory() string         { return p.directory }
func (p Plan) DataRoot() string          { return p.root }
func (p Plan) InstanceID() string        { return p.instanceID }
func (p Plan) InstancePath() string      { return path.Join("instances", p.instanceID) }

func (p Plan) Target() meta.Platform { return p.target }

// ProcessArguments is sensitive and must only be passed to os/exec. Keeping
// game arguments on the command line avoids writing access tokens to disk.
// Java 8 receives JVM options directly, with only its classpath replaced by a
// manifest-only JAR. Java 9+ retains its JVM argument-file transport.
func (p Plan) ProcessArguments(supportFile string) []string {
	args, _ := p.ProcessCommand(supportFile)
	return args
}

// ProcessCommand validates the real support path and direct exec arguments
// without exposing sensitive text in encoding failures.
func (p Plan) ProcessCommand(supportFile string) ([]string, error) {
	if p.mainIndex < 1 || p.mainIndex >= len(p.arguments) || p.javaMajor < 8 {
		return nil, errors.New("invalid launch plan")
	}
	if !safeText(supportFile) || supportFile == "" {
		return nil, errors.New("invalid JVM support path")
	}
	var args []string
	if p.javaMajor == 8 {
		index, err := p.classpathIndex()
		if err != nil {
			return nil, err
		}
		args = slices.Clone(p.arguments)
		args[index] = supportFile
	} else {
		args = append([]string{"@" + supportFile}, p.arguments[p.mainIndex:]...)
	}
	if err := java.ValidateNativeArguments(append([]string{p.executable}, args...), p.target.OS, p.nativeEncoding); err != nil {
		return nil, err
	}
	return args, nil
}

func (p Plan) JVMArgumentFile() ([]byte, error) {
	if p.javaMajor < 9 {
		return nil, errors.New("此 Java 不支持 JVM 参数文件")
	}
	if p.mainIndex < 1 || p.mainIndex >= len(p.arguments) {
		return nil, errors.New("invalid launch plan")
	}
	for _, arg := range p.arguments[:p.mainIndex] {
		for _, secret := range p.secrets {
			if secret != "" && strings.Contains(arg, secret) {
				return nil, errors.New("会话资料不能写入 JVM 参数文件")
			}
		}
	}
	return java.EncodeRuntimeArgumentFile(p.arguments[:p.mainIndex], p.target.OS, p.nativeEncoding)
}

func build(o Options, s *Session) (Preview, error) {
	if err := o.Game.Validate(); err != nil {
		return Preview{}, err
	}
	if !filepath.IsAbs(o.Root) || !filepath.IsAbs(o.Java.Executable) || !safeText(o.Root) || !safeText(o.Java.Executable) || !instance.ValidID(o.InstanceID) {
		return Preview{}, errors.New("启动目录、Java 路径或实例 ID 无效")
	}
	if o.GameDirectory != "" && (!filepath.IsAbs(o.GameDirectory) || !safeText(o.GameDirectory) || filepath.Clean(o.GameDirectory) != o.GameDirectory) {
		return Preview{}, errors.New("游戏工作目录无效")
	}
	if o.MaxMemoryMiB < 512 || o.MaxMemoryMiB > 65536 || o.Java.Arch == "386" && o.MaxMemoryMiB > 1536 {
		return Preview{}, errors.New("游戏内存超出当前 JVM 可用范围")
	}
	if o.MinMemoryMiB < 0 || o.MinMemoryMiB > o.MaxMemoryMiB {
		return Preview{}, errors.New("JVM 初始内存不能超过最大内存")
	}
	if err := ValidateJVMArguments(o.GCMode, o.JVMArguments); err != nil {
		return Preview{}, err
	}
	requirement := java.GameRequirement(o.Version.Java.Major, o.Platform)
	if len(o.JavaRequirement.Constraints) > 0 {
		var err error
		requirement, err = requirement.Intersect(o.JavaRequirement)
		if err != nil {
			return Preview{}, err
		}
	}
	if err := requirement.CheckRuntime(o.Java); err != nil {
		return Preview{}, err
	}
	if o.Java.Major < 8 {
		return Preview{}, errors.New("当前启动管线需要 Java 8 或更高版本")
	}
	if o.Version.Java.Major == 8 && o.Java.Major != 8 {
		return Preview{}, errors.New("此版本需要稳定的 Java 8，不能使用更高主版本替代")
	}
	parent := path.Join("instances", o.InstanceID, "natives")
	if !meta.ValidPath(o.NativeDirectory) || path.Dir(o.NativeDirectory) != parent || !strings.HasPrefix(path.Base(o.NativeDirectory), "ready-") || len(path.Base(o.NativeDirectory)) != len("ready-")+26 {
		return Preview{}, errors.New("启动本地库目录不属于当前实例")
	}
	if !validMainClass(o.Version.MainClass) {
		return Preview{}, errors.New("版本主类格式无效")
	}
	// P2's planner is the single source of library/native selection and order.
	plan, err := install.Build(o.Version, o.Platform)
	if err != nil {
		return Preview{}, err
	}
	absolute := func(relative string) string { return filepath.Join(o.Root, filepath.FromSlash(relative)) }
	separator := ":"
	if o.Platform.OS == "windows" {
		separator = ";"
	}
	var classpath []string
	for _, relative := range plan.Classpath {
		full := absolute(relative)
		if strings.Contains(full, separator) {
			return Preview{}, errors.New("游戏路径包含 classpath 分隔符，请更换数据目录")
		}
		if o.Java.Major == 8 {
			if _, err := classpathURL(full, o.Platform.OS); err != nil {
				return Preview{}, err
			}
		}
		classpath = append(classpath, full)
	}
	gameDirectory := o.GameDirectory
	if gameDirectory == "" {
		gameDirectory = absolute(path.Join("instances", o.InstanceID, "game"))
	}
	if o.LegacyAssetsDirectory != "" {
		parent := path.Join("instances", o.InstanceID, "legacy-assets")
		if !install.ValidLegacyAssetsDirectory(parent, o.LegacyAssetsDirectory) {
			return Preview{}, errors.New("历史资源目录不属于当前实例")
		}
	}
	values := map[string]string{
		"auth_player_name": "<登录后提供>", "auth_uuid": "<登录后提供>", "auth_access_token": "<登录后提供>", "auth_session": "<登录后提供>", "clientid": "<登录后提供>", "auth_xuid": "<登录后提供>",
		"user_type": "msa", "user_properties": "{}", "version_name": o.Version.ID, "version_type": o.Version.Type,
		"game_directory": gameDirectory, "assets_root": absolute("cache/assets"), "assets_index_name": o.Version.AssetIndex.ID,
		"natives_directory": absolute(o.NativeDirectory), "launcher_name": "mcl", "launcher_version": "0.1.0", "classpath": strings.Join(classpath, separator),
		"classpath_separator": separator, "library_directory": absolute("cache/libraries"),
	}
	if o.LegacyAssetsDirectory != "" {
		values["game_assets"] = absolute(path.Join(o.LegacyAssetsDirectory, "files"))
	}
	if o.Version.LibraryDirectory != "" {
		values["library_directory"] = absolute(o.Version.LibraryDirectory)
	}
	if o.Version.LaunchName != "" {
		values["version_name"] = o.Version.LaunchName
		// The physical client jar still uses the Mojang version ID.
		values["primary_jar_name"] = filepath.Base(plan.ClientPath)
		values["primary_jar"] = absolute(plan.ClientPath)
	}
	if s != nil {
		values["auth_player_name"], values["auth_uuid"], values["auth_access_token"], values["clientid"], values["auth_xuid"] = s.Name, s.UUID, s.token, s.ClientID, s.XUID
		values["auth_session"] = "token:" + s.token + ":" + s.UUID
		if s.offline {
			values["user_type"], values["auth_access_token"], values["auth_session"] = "legacy", "0", "0"
		}
	}
	platform := o.Platform
	// Only structured, validated settings may enable official feature rules.
	// In particular, never inherit arbitrary caller demo/quick-play flags.
	platform.Features = nil
	quickPlay := o.Game.AutoJoinServer && supportsQuickPlay(o.Version.Arguments.Game)
	gamePlatform := platform
	gamePlatform.Features = map[string]bool{
		"has_custom_resolution":     o.Game.Width != 0 && o.Game.Height != 0,
		"is_quick_play_multiplayer": quickPlay,
	}
	jvm, err := meta.SelectArguments(o.Version.Arguments.JVM, platform)
	if err != nil {
		return Preview{}, err
	}
	jvm, err = libraryDirectoryArguments(jvm, o.Version)
	if err != nil {
		return Preview{}, err
	}
	game, err := meta.SelectArguments(o.Version.Arguments.Game, gamePlatform)
	if err != nil {
		return Preview{}, err
	}
	if err := validateEntrypointTemplates(o.Version.MainClass, game); err != nil {
		return Preview{}, err
	}
	args := []string{fmt.Sprintf("-Xmx%dm", o.MaxMemoryMiB), "-Dfile.encoding=UTF-8"}
	if o.MinMemoryMiB > 0 {
		args = append(args, fmt.Sprintf("-Xms%dm", o.MinMemoryMiB))
	}
	collector, err := CollectorArguments(o.GCMode, o.Java)
	if err != nil {
		return Preview{}, err
	}
	args = append(args, collector...)
	args = append(args, o.JVMArguments...)
	for _, arg := range jvm {
		if strings.Contains(arg, "${auth_") || strings.Contains(arg, "${clientid}") {
			return Preview{}, errors.New("会话资料不能进入 JVM 参数文件")
		}
		value, err := substitute(arg, values)
		if err != nil {
			return Preview{}, err
		}
		if strings.HasPrefix(value, "-javaagent:") || strings.HasPrefix(value, "-agentlib:") || strings.HasPrefix(value, "-agentpath:") || strings.HasPrefix(value, "@") {
			return Preview{}, errors.New("当前原版管线不接受 Agent 或额外参数文件")
		}
		if reservedNativeEncodingOption(value) {
			return Preview{}, errors.New("版本不能覆盖Java原生字符编码")
		}
		if value == "--class-path" || strings.HasPrefix(value, "--class-path=") || value == "-Djava.class.path" || strings.HasPrefix(value, "-Djava.class.path=") {
			return Preview{}, errors.New("版本不能用其它参数覆盖classpath")
		}
		args = append(args, value)
	}
	if !slices.Contains(jvm, "-cp") && !slices.Contains(jvm, "-classpath") {
		return Preview{}, errors.New("版本缺少受支持的 classpath 参数")
	}
	if logging := o.Version.Logging.Client; logging != nil {
		arg, err := substitute(logging.Argument, map[string]string{"path": absolute(plan.LoggingPath)})
		if err != nil {
			return Preview{}, err
		}
		if arg != "-Dlog4j.configurationFile="+absolute(plan.LoggingPath) && arg != "-Dlog4j.configuration="+absolute(plan.LoggingPath) {
			return Preview{}, errors.New("当前管线不支持该日志参数")
		}
		args = append(args, arg)
	}
	gameArgs, err := gameArguments(game, values, o.Game, quickPlay)
	if err != nil {
		return Preview{}, err
	}
	if err := validateJVMEntrypoint(args); err != nil {
		return Preview{}, err
	}
	p := Plan{arguments: args, mainIndex: len(args)}
	index, err := p.classpathIndex()
	if err != nil {
		return Preview{}, err
	}
	if args[index] != values["classpath"] {
		return Preview{}, errors.New("classpath 必须使用已校验的运行库计划")
	}
	mainIndex := len(args)
	args = append(args, o.Version.MainClass)
	args = append(args, gameArgs...)
	return Preview{mainIndex: mainIndex, Priority: o.Game.Priority, Executable: o.Java.Executable, Directory: values["game_directory"], Arguments: args}, nil
}

func substitute(value string, values map[string]string) (string, error) {
	if !safeText(value) {
		return "", errors.New("启动参数包含无效字符")
	}
	var out strings.Builder
	for {
		prefix, rest, found := strings.Cut(value, "${")
		out.WriteString(prefix)
		if !found {
			return out.String(), nil
		}
		key, suffix, closed := strings.Cut(rest, "}")
		replacement, known := values[key]
		if key == "game_assets" && !known {
			return "", errors.New("此版本要求已准备的历史 game_assets 具名目录，不能用散列对象目录替代")
		}
		if !closed || !known {
			return "", fmt.Errorf("不支持的启动参数占位符 %q", key)
		}
		if !safeText(replacement) {
			return "", errors.New("启动参数替换值包含无效字符")
		}
		out.WriteString(replacement)
		value = suffix
	}
}

func safeText(value string) bool {
	return utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) == -1
}
func validMainClass(value string) bool {
	return regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$`).MatchString(value)
}
