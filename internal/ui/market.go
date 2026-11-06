package ui

import (
	"mcl/internal/app"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

type assetCategory struct {
	kind              app.AssetKind
	name, requirement string
	icon              *ui.SVG
}

var assetCategories = []assetCategory{
	{app.AssetMod, "模组", "需要兼容的模组加载器；当前原版实例尚未安装加载器。", iconPuzzle},
	{app.AssetShader, "光影", "需要支持光影的运行环境；仅放入文件不会为原版启用光影。", iconSparkles},
	{app.AssetResourcePack, "资源包", "需要匹配 Minecraft 版本，并在游戏内选择启用。", iconLayers},
	{app.AssetDataPack, "数据包", "按存档安装到 saves/<世界>/datapacks，而非实例根目录。", iconDatabase},
	{app.AssetModpack, "整合包", "创建独立实例并配置整合包要求的运行环境。", iconArchive},
}

func category(kind app.AssetKind) assetCategory {
	if kind == app.AssetKind(market.World) {
		return assetCategory{kind: kind, name: "地图", requirement: "地图会解压到独立存档，不覆盖现有世界", icon: iconMap}
	}
	for _, item := range assetCategories {
		if item.kind == kind {
			return item
		}
	}
	return assetCategories[0]
}
