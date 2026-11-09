package loader

import (
	"errors"

	"mcl/internal/core/meta"
)

const legacyForge17Tweaker = "cpw.mods.fml.common.launcher.FMLTweaker"

func legacyForgeTweakerFor(request Request) (string, bool) {
	if request.Type != "forge" {
		return "", false
	}
	switch request.GameVersion {
	case "1.8", "1.8.1", "1.8.2", "1.8.3", "1.8.4", "1.8.5", "1.8.6", "1.8.7", "1.8.8", "1.8.9",
		"1.9", "1.9.1", "1.9.2", "1.9.3", "1.9.4", "1.10", "1.10.1", "1.10.2",
		"1.11", "1.11.1", "1.11.2", "1.12", "1.12.1", "1.12.2":
		return legacyForgeTweaker, true
	case "1.6.1", "1.6.2", "1.6.4", "1.7.2", "1.7.10":
		return legacyForge17Tweaker, true
	default:
		return "", false
	}
}
func legacyForgeWrapper(request Request, name string) bool {
	if _, supported := legacyForgeTweakerFor(request); !supported {
		return false
	}
	if name == "net.minecraft:launchwrapper:1.12" {
		return true
	}
	switch request.GameVersion {
	case "1.6.1", "1.6.2", "1.6.4":
		return name == "net.minecraft:launchwrapper:1.3" || name == "net.minecraft:launchwrapper:1.8"
	case "1.7.2":
		return name == "net.minecraft:launchwrapper:1.8" || name == "net.minecraft:launchwrapper:1.9" || name == "net.minecraft:launchwrapper:1.11"
	case "1.7.10":
		return name == "net.minecraft:launchwrapper:1.11"
	}
	return false
}

// A second, unmatched core/wrapper must not silently win the classpath. The
// approved entry libraries cannot be conditionally omitted, including caches.
func validateLegacyRuntimeLibrary(request Request, library meta.Library, processed bool) error {
	coordinate, err := meta.ParseCoordinate(library.Name)
	if err != nil {
		return err
	}
	core := coordinate.Group == "net.minecraftforge" && coordinate.Artifact == "forge"
	wrapper := coordinate.Group == "net.minecraft" && coordinate.Artifact == "launchwrapper"
	coreAllowed := legacyForgeCore(request, library.Name)
	if processed {
		coreAllowed = processedLegacyCore(request, library.Name)
	}
	if core && !coreAllowed || wrapper && !legacyForgeWrapper(request, library.Name) {
		return errors.New("旧Forge包含另一个版本或未知分类的核心/LaunchWrapper")
	}
	if (core || wrapper) && len(library.Rules) != 0 {
		return errors.New("旧Forge核心/LaunchWrapper不能条件省略")
	}
	return nil
}
