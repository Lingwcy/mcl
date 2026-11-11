package launch

import (
	"errors"
	"fmt"
	"strings"
)

// JVM option operands may happen to spell the eventual main class. They must
// not be searched as an entrypoint, nor may metadata select a JAR/module/main
// before the one fixed by the validated version.
func validateJVMEntrypoint(arguments []string) error {
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		key, _, inline := strings.Cut(arg, "=")
		switch key {
		case "-jar", "-m", "--module", "--source", "--dry-run", "--version", "-version", "--help", "-help", "-h", "--help-extra", "-fullversion", "--full-version", "-?", "-X", "--list-modules", "-d", "--describe-module", "--validate-modules", "--":
			return errors.New("JVM模板不能替换或跳过固定客户端入口")
		case "-cp", "-classpath", "-p", "--module-path", "--upgrade-module-path", "--add-modules", "--limit-modules", "--add-reads", "--add-exports", "--add-opens", "--patch-module", "--enable-native-access":
			if inline {
				continue
			}
			if i+1 >= len(arguments) || arguments[i+1] == "" || strings.HasPrefix(arguments[i+1], "-") {
				return errors.New("JVM模板缺少选项值")
			}
			i++
		default:
			if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "@") {
				return errors.New("JVM模板包含额外主类或参数文件")
			}
		}
	}
	return nil
}

// Inspect selected templates BEFORE substitution: an account name or token
// spelling a flag is data, never evidence of an extra launcher option. Rules
// have already removed inactive variants; no argument is silently discarded.
func validateEntrypointTemplates(main string, arguments []string) error {
	single := map[string]bool{
		"--username": true, "--uuid": true, "--accessToken": true, "--session": true,
		"--clientId": true, "--xuid": true, "--version": true, "--gameDir": true,
		"--assetsDir": true, "--assetIndex": true, "--userType": true,
		"--versionType": true, "--userProperties": true, "--launchTarget": true,
	}
	seen := make(map[string]bool)
	tweakers := make(map[string]bool)
	for i := 0; i < len(arguments); i++ {
		key, value, inline := strings.Cut(arguments[i], "=")
		if !single[key] && key != "--tweakClass" {
			continue
		}
		if !inline {
			if i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "--") {
				return fmt.Errorf("游戏入口参数 %s 缺少选项值", key)
			}
			i++
			value = arguments[i]
		}
		if value == "" {
			return fmt.Errorf("游戏入口参数 %s 缺少选项值", key)
		}
		if key == "--tweakClass" {
			if main != "net.minecraft.launchwrapper.Launch" || !validMainClass(value) {
				return errors.New("Tweaker与LaunchWrapper主类不匹配或不是固定类名")
			}
			if tweakers[value] {
				return errors.New("同一Tweaker被重复加入启动模板")
			}
			tweakers[value] = true
			continue
		}
		if seen[key] {
			return fmt.Errorf("游戏启动模板重复声明 %s", key)
		}
		seen[key] = true
	}
	if seen["--session"] && (seen["--accessToken"] || seen["--uuid"]) {
		return errors.New("游戏模板混用了旧会话与现代身份参数")
	}
	if main == "net.minecraft.launchwrapper.Launch" {
		if len(tweakers) == 0 || seen["--launchTarget"] {
			return errors.New("LaunchWrapper缺少Tweaker或混入另一代入口")
		}
	}
	return nil
}
