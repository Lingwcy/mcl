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
	case "1.12.2":
		return legacyForgeTweaker, true
	case "1.7.10":
		return legacyForge17Tweaker, true
	default:
		return "", false
	}
}
func legacyForgeWrapper(request Request, name string) bool {
	if _, supported := legacyForgeTweakerFor(request); !supported {
		return false
	}
	return name == "net.minecraft:launchwrapper:1.12" || request.GameVersion == "1.7.10" && name == "net.minecraft:launchwrapper:1.11"
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
