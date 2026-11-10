package loader

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

func processedLegacyCore(request Request, name string) bool {
	if _, supported := legacyForgeTweakerFor(request); !supported {
		return false
	}
	if legacyForgeCore(request, name) {
		return true
	}
	coordinate, err := meta.ParseCoordinate(name)
	return err == nil && request.Type == "forge" && coordinate.Group == "net.minecraftforge" && coordinate.Artifact == "forge" && coordinate.Version == request.GameVersion+"-"+strings.TrimPrefix(request.Version, request.GameVersion+"-") && coordinate.Classifier == "client" && coordinate.Extension == "jar"
}
func normalizeProcessedLegacyVersion(request Request, version *meta.Version) error {
	if !legacyForgeID(request, version.ID) || version.MainClass != legacyForgeMain || version.Java.Major != 0 && version.Java.Major != 8 || version.Jar != "" && version.Jar != request.GameVersion || len(version.Patches) != 0 || len(version.Downloads) != 0 || version.AssetIndex.ID != "" {
		return errors.New("安装器的旧游戏配置不在已支持LaunchWrapper范围")
	}
	args, err := legacyForgeArguments(request, version.LegacyArguments)
	if err != nil {
		return err
	}
	version.Arguments = &args
	version.LegacyArguments = ""
	version.Java.Major = 8
	return nil
}
func validateProcessedLegacyGame(profile Profile) error {
	if profile.Legacy || !legacyForgeID(profile.Request, profile.ID) || profile.MainClass != legacyForgeMain || profile.JavaMajor != 8 || len(profile.Arguments.JVM) != 0 {
		return errors.New("旧游戏客户端模板身份不匹配")
	}
	var values []string
	for _, argument := range profile.Arguments.Game {
		if len(argument.Rules) != 0 || len(argument.Values) != 1 {
			return errors.New("处理器旧游戏模板不能包含条件参数")
		}
		values = append(values, argument.Values[0])
	}
	if err := validateLegacyForgeArguments(profile.Request, values); err != nil {
		return err
	}
	core, wrapper := 0, 0
	for _, library := range profile.Libraries {
		if err := validateLegacyRuntimeLibrary(profile.Request, library, true); err != nil {
			return err
		}
		if processedLegacyCore(profile.Request, library.Name) {
			if len(library.Rules) != 0 {
				return errors.New("Forge核心客户端不能条件省略")
			}
			core++
		}
		if legacyForgeWrapper(profile.Request, library.Name) {
			if len(library.Rules) != 0 {
				return errors.New("LaunchWrapper入口不能条件省略")
			}
			wrapper++
		}
	}
	if core != 1 || wrapper != 1 {
		return errors.New("安装器缺少唯一匹配的Forge核心或LaunchWrapper")
	}
	return nil
}
func verifyProcessedLegacyEntrypoints(ctx context.Context, stage *os.Root, profile Profile) error {
	if !profile.LegacyGame {
		return nil
	}
	for _, library := range profile.Libraries {
		entry := ""
		if processedLegacyCore(profile.Request, library.Name) {
			tweaker, _ := legacyForgeTweakerFor(profile.Request)
			entry = strings.ReplaceAll(tweaker, ".", "/") + ".class"
		}
		if legacyForgeWrapper(profile.Request, library.Name) {
			entry = "net/minecraft/launchwrapper/Launch.class"
		}
		if entry == "" {
			continue
		}
		identity := library.Downloads.Artifact
		name := path.Join("libraries", identity.Path)
		if _, err := hashForgeFile(ctx, stage, name, identity.SHA1, identity.Size); err != nil {
			return err
		}
		f, err := stage.Open(name)
		if err != nil {
			return err
		}
		content, err := archive.Open(f, identity.Size)
		if err == nil && !content.Has(entry) {
			err = errors.New("Forge客户端运行库缺少固定启动入口")
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
