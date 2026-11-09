package discovery

import (
	"encoding/json"
	"errors"
	"strings"

	"mcl/internal/core/meta"
)

func parseDocument(data []byte) (document, error) {
	var value document
	if err := json.Unmarshal(data, &value); err != nil {
		return value, errors.New("版本JSON损坏或字段格式不受支持")
	}
	if !meta.ValidID(value.ID) || value.InheritsFrom != "" && !meta.ValidID(value.InheritsFrom) || value.Jar != "" && !meta.ValidID(value.Jar) {
		return value, errors.New("版本ID、父版本或客户端引用不安全")
	}
	if len(value.MainClass) > 512 || len(value.Type) > 32 || len(value.Downloads) > 64 {
		return value, errors.New("版本显示字段或下载声明超过边界")
	}
	if len(value.Libraries) > 4096 || len(value.Patches) > 32 || len(value.MinecraftArguments) > 64<<10 {
		return value, errors.New("版本声明数量超过边界")
	}
	return value, nil
}
func loaderHints(value document) ([]Loader, bool) {
	var result []Loader
	unknown := false
	add := func(kind, version string) {
		if version == "" || len(version) > 160 {
			unknown = true
			return
		}
		for _, previous := range result {
			if previous.Kind == kind && previous.Version == version {
				return
			}
		}
		result = append(result, Loader{kind, version})
	}
	for _, library := range value.Libraries {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			unknown = true
			continue
		}
		switch coordinate.Group + ":" + coordinate.Artifact {
		case "net.fabricmc:fabric-loader":
			add("fabric", coordinate.Version)
		case "org.quiltmc:quilt-loader":
			add("quilt", coordinate.Version)
		case "net.minecraftforge:forge":
			add("forge", coordinate.Version)
		case "net.neoforged:forge", "net.neoforged:neoforge":
			add("neoforge", coordinate.Version)
		case "optifine:OptiFine":
			add("optifine", coordinate.Version)
			unknown = true
		}
	}
	for _, patch := range value.Patches {
		switch strings.ToLower(patch.ID) {
		case "game", "minecraft", "net.minecraft":
		case "forge", "neoforge", "fabric", "quilt":
			add(strings.ToLower(patch.ID), patch.Version)
		default:
			unknown = true
		}
	}
	if value.MainClass != "" && value.MainClass != "net.minecraft.client.main.Main" && value.MainClass != "net.minecraft.client.Minecraft" && value.MainClass != "net.minecraft.launchwrapper.Launch" && value.MainClass != "cpw.mods.modlauncher.Launcher" && value.MainClass != "cpw.mods.bootstraplauncher.BootstrapLauncher" && value.MainClass != "net.minecraftforge.bootstrap.ForgeBootstrap" && value.MainClass != "net.neoforged.fml.startup.Client" && !strings.HasPrefix(value.MainClass, "net.fabricmc.loader.") && !strings.HasPrefix(value.MainClass, "org.quiltmc.loader.") {
		unknown = true
	}
	return result, unknown
}
