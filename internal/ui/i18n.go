package ui

import (
	"strings"

	"mcl/internal/app"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type uiLanguageOption struct {
	code  string
	label string
}

var uiLanguageOptions = []uiLanguageOption{
	{code: app.LangEn, label: "英语 - English"},
	{code: app.LangJa, label: "日语 - 日本語"},
	{code: app.LangZhHans, label: "简体中文"},
	{code: app.LangZhHant, label: "繁體中文"},
}

func uiLanguageLabel(code string) string {
	for _, opt := range uiLanguageOptions {
		if opt.code == code {
			return opt.label
		}
	}
	return "简体中文"
}

func (s *Shell) effectiveI18n() app.I18nSettings {
	cfg := s.i18n
	if cfg.Language == "" && s.locale != "" {
		cfg.Language = s.locale
	}
	return cfg
}

func (s *Shell) selectLanguage(code string) {
	norm, ok := app.NormalizeLanguage(code)
	if !ok || norm == "" {
		return
	}
	s.locale = norm
	s.i18n.Language = norm
	if s.window != nil {
		mygo.App.SetLocale(norm)
	}
	s.saveSettings()
}

func (s *Shell) tr(text string) string {
	return translateText(s.locale, text)
}

func tr(c *ui.Context, text string) string {
	if c == nil {
		return text
	}
	locale := *ui.Local(c.Root(), "mcl-locale", func() string { return app.LangZhHans })
	return translateText(locale, text)
}

func translateText(locale, text string) string {
	if text == "" || locale == "" || locale == app.LangZhHans {
		return text
	}
	switch locale {
	case app.LangEn:
		if out, ok := enTranslations[text]; ok {
			return out
		}
		return translateDynamicEN(text)
	case app.LangJa:
		if out, ok := jaTranslations[text]; ok {
			return out
		}
		return translateDynamicJA(text)
	case app.LangZhHant:
		if out, ok := zhHantTranslations[text]; ok {
			return out
		}
		return toTraditionalChinese(text)
	default:
		return text
	}
}

func (s *Shell) i18nSettingsView(c *ui.Context) {
	t := s.theme
	p := colors(t)
	blocked := s.loadErr != nil || s.save == nil

	ui.Scroll(c.Key("i18n-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
		ui.Column(c.Key("i18n-language-section")).FillWidth().Shrink(0).Gap(8).Children(func() {
			settingsGroupLabel(c, s.tr("语言"))
			ui.Column(c.Key("i18n-language-card")).FillWidth().Shrink(0).Radius(10).
				Border(0.5, p.ListDivider).Background(t.Surface).Clip().Children(func() {
				ui.Row(c.Key("i18n-language-row")).FillWidth().Height(46).Shrink(0).Padding(0, 14).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconLanguages).Size(15, 15).Shrink(0).TextColor(p.Icon)
					ui.Text(c, s.tr("语言 (Language)")).FontSize(12).Grow(1).MinWidth(0)
					btn := ui.ButtonBase(c.Key("i18n-language-select")).Label(s.tr("语言 (Language)")).
						Height(28).Radius(6).Padding(0, 10).Gap(8).Disabled(blocked)
					fill := p.Control
					if !blocked && btn.Hovered() {
						fill = p.Hover
					}
					rowMotion(c, btn, fill).Menu(func(menu *ui.Menu) {
						for _, opt := range uiLanguageOptions {
							if menu.Item(opt.label).Checked(s.locale == opt.code).Chosen() {
								s.selectLanguage(opt.code)
							}
						}
					}).Children(func() {
						ui.Text(c, uiLanguageLabel(s.locale)).FontSize(12).SingleLine()
						ui.Icon(c, iconChevronDown).Size(12, 12).Shrink(0).TextColor(p.Metadata)
					})
				})
			})
		})

		ui.Column(c.Key("i18n-options-card")).FillWidth().Shrink(0).Radius(10).
			Border(0.5, p.ListDivider).Background(t.Surface).Clip().Children(func() {
			i18nToggleRow(c, "i18n-auto-instance-lang",
				s.tr("自动设置实例语言"),
				s.tr("启用后，新建实例的语言将自动设置为启动器语言，这将同时跳过游戏内辅助功能设置"),
				&s.i18n.AutoSetInstanceLanguage, blocked, s.saveSettings)
			gameSettingsDivider(c)
			i18nToggleRow(c, "i18n-translate-resources",
				s.tr("显示资源翻译"),
				s.tr("启用后，在资源下载界面尝试显示名称或描述的简体中文翻译（如数据可用）"),
				&s.i18n.TranslateResources, blocked, s.saveSettings)
			gameSettingsDivider(c)
			i18nToggleRow(c, "i18n-prefix-filename",
				s.tr("资源文件名添加翻译前缀"),
				s.tr("启用后，下载与更新资源时将在文件名添加简体中文译名前缀（如“[地图] map.jar”）"),
				&s.i18n.PrefixResourceFileName, blocked, s.saveSettings)
		})
	})
}

func i18nToggleRow(c *ui.Context, key, title, subtitle string, value *bool, disabled bool, onToggle func()) {
	t := c.Theme()
	p := colors(t)
	ui.Row(c.Key(key)).FillWidth().Shrink(0).Padding(11, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(500).TextColor(t.Text)
			ui.Text(c, subtitle).FontSize(11).TextColor(p.Metadata).MaxLines(2)
		})
		ui.Switch(c.Key(key+"-switch"), value).Label(title).Size(32, 18).Shrink(0).Disabled(disabled).OnClick(onToggle)
	})
}

func translateDynamicEN(text string) string {
	switch {
	case strings.HasPrefix(text, "显示全部 (") && strings.HasSuffix(text, ")"):
		return "Show All (" + strings.TrimSuffix(strings.TrimPrefix(text, "显示全部 ("), ")") + ")"
	case strings.HasPrefix(text, "使用墙纸 "):
		return "Use Wallpaper " + strings.TrimPrefix(text, "使用墙纸 ")
	case strings.HasPrefix(text, "移除 "):
		return "Remove " + strings.TrimPrefix(text, "移除 ")
	case strings.HasPrefix(text, "版本 "):
		return "Version " + strings.TrimPrefix(text, "版本 ")
	case strings.HasPrefix(text, "下载中 "):
		return "Downloading " + strings.TrimPrefix(text, "下载中 ")
	case strings.HasSuffix(text, " 更新内容"):
		return strings.TrimSuffix(text, " 更新内容") + " Release Notes"
	case strings.HasPrefix(text, "新版本 ") && strings.HasSuffix(text, " 可用"):
		return "Update " + strings.TrimSuffix(strings.TrimPrefix(text, "新版本 "), " 可用") + " available"
	case strings.HasPrefix(text, "正在下载 "):
		return "Downloading " + strings.TrimPrefix(text, "正在下载 ")
	case strings.HasPrefix(text, "新版本 ") && strings.HasSuffix(text, " 已下载，重启后生效"):
		return "Update " + strings.TrimSuffix(strings.TrimPrefix(text, "新版本 "), " 已下载，重启后生效") + " ready; restart to apply"
	case strings.HasPrefix(text, "切换到 "):
		return "Switch to " + strings.TrimPrefix(text, "切换到 ")
	case strings.HasSuffix(text, " 的账户选项"):
		return "Account options for " + strings.TrimSuffix(text, " 的账户选项")
	case strings.HasSuffix(text, " 的更多操作"):
		return "More actions for " + strings.TrimSuffix(text, " 的更多操作")
	case strings.HasPrefix(text, "启动并加入 "):
		return "Launch and join " + strings.TrimPrefix(text, "启动并加入 ")
	case strings.HasPrefix(text, "启动实例："):
		return "Launch instance: " + strings.TrimPrefix(text, "启动实例：")
	case strings.HasPrefix(text, "服务端："):
		return "Server: " + strings.TrimPrefix(text, "服务端：")
	case strings.HasPrefix(text, "排序："):
		return "Sort: " + strings.TrimPrefix(text, "排序：")
	case strings.HasPrefix(text, "账户 · "):
		return "Account · " + strings.TrimPrefix(text, "账户 · ")
	case strings.HasPrefix(text, "下载量 "):
		return "Downloads " + strings.TrimPrefix(text, "下载量 ")
	case strings.HasPrefix(text, "查看版本 "):
		return "View version " + strings.TrimPrefix(text, "查看版本 ")
	case strings.HasPrefix(text, "退出代码 "):
		return "Exit code " + strings.TrimPrefix(text, "退出代码 ")
	case strings.HasPrefix(text, "第 ") && strings.HasSuffix(text, " 页"):
		return "Page " + strings.TrimSuffix(strings.TrimPrefix(text, "第 "), " 页")
	case strings.HasSuffix(text, " 正在运行中"):
		return strings.TrimSuffix(text, " 正在运行中") + " is already running"
	case strings.HasSuffix(text, " 项"):
		return strings.TrimSuffix(text, " 项") + " items"
	case strings.HasPrefix(text, "准备启动游戏 · "):
		return "Preparing launch · " + strings.TrimPrefix(text, "准备启动游戏 · ")
	case strings.HasPrefix(text, "启用 "):
		return "Enable " + strings.TrimPrefix(text, "启用 ")
	case strings.HasPrefix(text, "选择版本 "):
		return "Select version " + strings.TrimPrefix(text, "选择版本 ")
	case strings.HasPrefix(text, "加载器 "):
		return "Loader " + strings.TrimPrefix(text, "加载器 ")
	case strings.HasPrefix(text, "图标 "):
		return "Icon " + strings.TrimPrefix(text, "图标 ")
	case strings.HasSuffix(text, "实例"):
		base := strings.TrimSuffix(text, "实例")
		if base != "" {
			if trBase, ok := enTranslations[base]; ok {
				return trBase + " Instance"
			}
		}
	}
	return text
}

func translateDynamicJA(text string) string {
	switch {
	case strings.HasPrefix(text, "显示全部 (") && strings.HasSuffix(text, ")"):
		return "すべて表示 (" + strings.TrimSuffix(strings.TrimPrefix(text, "显示全部 ("), ")") + ")"
	case strings.HasPrefix(text, "使用墙纸 "):
		return "壁紙を使用 " + strings.TrimPrefix(text, "使用墙纸 ")
	case strings.HasPrefix(text, "移除 "):
		return "削除 " + strings.TrimPrefix(text, "移除 ")
	case strings.HasPrefix(text, "启用 "):
		return "有効化 " + strings.TrimPrefix(text, "启用 ")
	case strings.HasPrefix(text, "版本 "):
		return "バージョン " + strings.TrimPrefix(text, "版本 ")
	case strings.HasPrefix(text, "选择版本 "):
		return "バージョン選択 " + strings.TrimPrefix(text, "选择版本 ")
	case strings.HasPrefix(text, "加载器 "):
		return "ローダー " + strings.TrimPrefix(text, "加载器 ")
	case strings.HasPrefix(text, "图标 "):
		return "アイコン " + strings.TrimPrefix(text, "图标 ")
	case strings.HasPrefix(text, "下载中 "):
		return "ダウンロード中 " + strings.TrimPrefix(text, "下载中 ")
	case strings.HasSuffix(text, " 更新内容"):
		return strings.TrimSuffix(text, " 更新内容") + " リリースノート"
	case strings.HasPrefix(text, "新版本 ") && strings.HasSuffix(text, " 可用"):
		return "新バージョン " + strings.TrimSuffix(strings.TrimPrefix(text, "新版本 "), " 可用") + " が利用可能"
	case strings.HasPrefix(text, "正在下载 "):
		return "ダウンロード中: " + strings.TrimPrefix(text, "正在下载 ")
	case strings.HasPrefix(text, "新版本 ") && strings.HasSuffix(text, " 已下载，重启后生效"):
		return "新バージョン " + strings.TrimSuffix(strings.TrimPrefix(text, "新版本 "), " 已下载，重启后生效") + " の準備完了（再起動で適用）"
	case strings.HasPrefix(text, "切换到 "):
		return strings.TrimPrefix(text, "切换到 ") + " に切り替え"
	case strings.HasSuffix(text, " 的账户选项"):
		return strings.TrimSuffix(text, " 的账户选项") + " のアカウント設定"
	case strings.HasSuffix(text, " 的更多操作"):
		return strings.TrimSuffix(text, " 的更多操作") + " の詳細操作"
	case strings.HasPrefix(text, "启动并加入 "):
		return "起動して参加: " + strings.TrimPrefix(text, "启动并加入 ")
	case strings.HasPrefix(text, "启动实例："):
		return "起動インスタンス：" + strings.TrimPrefix(text, "启动实例：")
	case strings.HasPrefix(text, "服务端："):
		return "サーバー：" + strings.TrimPrefix(text, "服务端：")
	case strings.HasPrefix(text, "排序："):
		return "並び替え：" + strings.TrimPrefix(text, "排序：")
	case strings.HasPrefix(text, "账户 · "):
		return "アカウント · " + strings.TrimPrefix(text, "账户 · ")
	case strings.HasPrefix(text, "下载量 "):
		return "ダウンロード数 " + strings.TrimPrefix(text, "下载量 ")
	case strings.HasPrefix(text, "查看版本 "):
		return "バージョン詳細: " + strings.TrimPrefix(text, "查看版本 ")
	case strings.HasPrefix(text, "退出代码 "):
		return "終了コード " + strings.TrimPrefix(text, "退出代码 ")
	case strings.HasPrefix(text, "第 ") && strings.HasSuffix(text, " 页"):
		return strings.TrimSuffix(strings.TrimPrefix(text, "第 "), " 页") + " ページ"
	case strings.HasSuffix(text, " 正在运行中"):
		return strings.TrimSuffix(text, " 正在运行中") + " はすでに実行中です"
	case strings.HasSuffix(text, " 项"):
		return strings.TrimSuffix(text, " 项") + " 件"
	case strings.HasPrefix(text, "准备启动游戏 · "):
		return "起動準備中 · " + strings.TrimPrefix(text, "准备启动游戏 · ")
	case strings.HasSuffix(text, "实例"):
		base := strings.TrimSuffix(text, "实例")
		if base != "" {
			if trBase, ok := jaTranslations[base]; ok {
				return trBase + " インスタンス"
			}
		}
	}
	return text
}

var enTranslations = map[string]string{
	// Navigation & Titlebar
	"主导航":     "Main Navigation",
	"副侧栏":     "Secondary Sidebar",
	"设置导航":    "Settings Navigation",
	"主页":      "Home",
	"实例":      "Instances",
	"市场":      "Market",
	"服务器":     "Servers",
	"设置":      "Settings",
	"收起或展开侧栏": "Toggle Sidebar",
	"游戏库":     "Library",
	"下载与任务":   "Downloads & Tasks",
	"切换明暗主题":  "Toggle Appearance",
	"最小化窗口":   "Minimize Window",
	"最大化窗口":   "Maximize Window",
	"还原窗口":    "Restore Window",
	"关闭窗口":    "Close Window",
	"暂无实例":    "No instances",
	"搜索实例":    "Search Instances",
	"新建实例":    "New Instance",

	// Settings Sidebar
	"游戏设置":    "Game Settings",
	"个性化":     "Personalization",
	"国际化":     "Internationalization",
	"下载设置":    "Downloads",
	"Java 管理": "Java Management",
	"账户":      "Accounts",
	"关于与更新":   "About & Updates",
	"鸣谢":      "Credits",

	// Internationalization Page
	"语言":            "Language",
	"语言 (Language)": "Language",
	"自动设置实例语言":      "Auto-set Instance Language",
	"启用后，新建实例的语言将自动设置为启动器语言，这将同时跳过游戏内辅助功能设置": "Automatically sets newly created or launched instances to the launcher language and skips the in-game accessibility onboarding screen",
	"显示资源翻译": "Show Resource Translations",
	"启用后，在资源下载界面尝试显示名称或描述的简体中文翻译（如数据可用）": "Attempts to show translated names and descriptions in the resource market when available",
	"资源文件名添加翻译前缀": "Prefix Resource Filenames",
	"启用后，下载与更新资源时将在文件名添加简体中文译名前缀（如“[地图] map.jar”）": "Prepends a localized tag prefix to downloaded and updated resource filenames (e.g. \"[地图] map.jar\")",

	// Home View
	"个性化设置":    "Personalization",
	"随机换一张墙纸":  "Shuffle Wallpaper",
	"未选择角色":    "No Account Selected",
	"点击添加账户":   "Click to Add Account",
	"离线模式":     "Offline Mode",
	"正版账户":     "Microsoft Account",
	"打开账户管理":   "Open Account Manager",
	"切换账户":     "Switch Account",
	"离线":       "Offline",
	"微软":       "Microsoft",
	"账户管理…":    "Manage Accounts…",
	"启动游戏":     "Launch Game",
	"尚未选择游戏实例": "No instance selected",
	"选择游戏实例":   "Select Instance",
	"切换或新建实例":  "Switch or Create Instance",
	"新建实例…":    "New Instance…",
	"管理全部实例…":  "Manage Instances…",

	// Personalization Page
	"我的墙纸": "My Wallpapers",
	"收起":   "Collapse",
	"添加墙纸": "Add Wallpaper",
	"从本地选择一张或多张图片作为墙纸": "Choose one or more local images as wallpapers",
	"混合随机切换": "Shuffle Wallpapers",
	"每次启动随机切换，点击立即换一张": "Cycle randomly across your wallpapers; click to switch now",
	"mcl 默认壁纸":   "mcl Default Wallpaper",
	"mcl 动漫默认壁纸": "mcl Default Anime Wallpaper",
	"默认":         "Default",
	"设为墙纸":       "Set as Wallpaper",
	"移除墙纸":       "Remove Wallpaper",
	"背景效果":       "Background Effects",
	"恢复默认":       "Reset",
	"恢复默认效果":     "Reset Effects",
	"换一张":        "Next",
	"背景不透明度":     "Opacity",
	"背景模糊":       "Blur",
	"背景压暗":       "Dimming",
	"已添加墙纸":      "Wallpaper added",
	"已移除墙纸":      "Wallpaper removed",

	// Game Settings
	"分辨率":        "Resolution",
	"游戏窗口宽度":     "Game Window Width",
	"游戏窗口高度":     "Game Window Height",
	"自定义":        "Custom",
	"游戏分辨率":      "Game Resolution",
	"全屏启动":       "Fullscreen",
	"自定义信息":      "Custom Info",
	"自定义游戏信息":    "Custom Game Info",
	"自动加入服务器":    "Auto-Join Server",
	"服务器地址":      "Server Address",
	"游戏内存":       "Game Memory",
	"自动分配":       "Automatic",
	"进程优先级":      "Process Priority",
	"游戏进程优先级":    "Game Process Priority",
	"启动后显示日志":    "Show Logs on Launch",
	"启动后显示游戏日志":  "Show Game Logs on Launch",
	"内存管理":       "Garbage Collector",
	"JVM 内存管理":   "JVM Garbage Collector",
	"JVM 参数":     "JVM Arguments",
	"自定义 JVM 参数": "Custom JVM Arguments",
	"无":          "None",
	"尽量使用 ZGC":   "Prefer ZGC",
	"尽量使用分代 ZGC": "Prefer Generational ZGC",
	"标准 G1GC":    "Standard G1GC",
	"调优 G1GC":    "Tuned G1GC",
	"不指定（可自定义）":  "Unspecified (Custom)",
	"未知策略":       "Unknown Policy",
	"低":          "Low",
	"低于正常":       "Below Normal",
	"正常":         "Normal",
	"高于正常":       "Above Normal",
	"高":          "High",
	"游戏目录":       "Game Directory",

	// Download Settings
	"下载源":             "Download Source",
	"下载源策略":           "Download Source Policy",
	"并发下载":            "Concurrency",
	"自定义下载并发数":        "Custom Download Concurrency",
	"速度限制":            "Speed Limit",
	"自定义下载速率上限，KiB/s": "Custom Download Speed Limit (KiB/s)",
	"下载速率上限":          "Download Speed Limit",
	"下载代理":            "Proxy",
	"下载代理类型":          "Download Proxy Mode",
	"代理地址":            "Proxy Address",
	"代理主机":            "Proxy Host",
	"主机":              "Host",
	"代理端口":            "Proxy Port",
	"端口":              "Port",
	"仅官方":             "Official Only",
	"自动":              "Auto",
	"官方优先":            "Official First",
	"镜像优先":            "Mirror First",
	"环境变量":            "Environment",
	"直连":              "Direct",
	"不限速":             "Unlimited",
	"未知":              "Unknown",

	// Java Management
	"返回 Java 管理":    "Back to Java Management",
	"下载 Java":       "Download Java",
	"刷新下载目录":        "Refresh Catalog",
	"下载":            "Download",
	"添加":            "Add",
	"添加本机 Java":     "Add Local Java",
	"选择 Java 目录":    "Select Java Directory",
	"选择可执行文件":       "Select Executable",
	"重新发现本机 Java":   "Rescan Local Java",
	"正在发现 Java…":    "Discovering Java…",
	"正在检查 Java…":    "Inspecting Java…",
	"尚未发现 Java":     "No Java Found",
	"Java 主版本":      "Java Major Version",
	"读取 Java 官方目录":  "Loading Official Java Catalog",
	"此版本暂无适合本机的发行包": "No compatible packages available for this platform",
	"安装":            "Install",
	"已安装":           "Installed",
	"已取消":           "Canceled",
	"已暂停":           "Paused",
	"正在取消…":         "Canceling…",
	"查看":            "View",
	"关闭 Java 任务":    "Dismiss Java Task",
	"取消安装 Java":     "Cancel Java Install",
	"全局默认 Java":     "Global Default Java",
	"更多操作":          "More Actions",
	"设为全局默认":        "Set as Global Default",
	"打开所在文件夹":       "Open Containing Folder",
	"复制 Java 路径":    "Copy Java Path",
	"从列表移除":         "Remove from List",

	// Accounts
	"正在读取账户":   "Loading Accounts",
	"重新读取账户":   "Reload Accounts",
	"正在添加离线账户": "Adding Offline Account",
	"添加账户":     "Add Account",
	"微软账户":     "Microsoft Account",
	"离线 ID":    "Offline ID",
	"还没有账户":    "No Accounts Yet",
	"登录微软账户":   "Sign in with Microsoft",
	"添加离线 ID":  "Add Offline ID",
	"微软登录需要配置 mcl 自有的应用 Client ID": "Microsoft sign-in requires configuring mcl's Client ID",
	"取消登录":      "Cancel Sign-in",
	"离线玩家名称":    "Offline Player Name",
	"玩家名称":      "Player Name",
	"关闭离线账户表单":  "Close Offline Form",
	"当前账户":      "Active Account",
	"账户选项":      "Account Options",
	"重新登录":      "Re-authenticate",
	"使用同名正版头像":  "Fetch Skin Avatar by Name",
	"从本地皮肤设置头像": "Set Avatar from Local Skin",
	"移除账户":      "Remove Account",

	// Market
	"模组":                    "Mods",
	"光影":                    "Shaders",
	"资源包":                   "Resource Packs",
	"数据包":                   "Data Packs",
	"整合包":                   "Modpacks",
	"地图":                    "Worlds",
	"不指定实例":                 "Any Instance",
	"选择目标实例":                "Select Target Instance",
	"搜索市场":                  "Search Market",
	"搜索":                    "Search",
	"全部版本":                  "All Versions",
	"筛选游戏版本":                "Filter Game Version",
	"市场排序":                  "Sort Order",
	"清除筛选":                  "Clear Filters",
	"配置 CurseForge API Key": "Configure CurseForge API Key",
	"需要 CurseForge API Key": "CurseForge API Key Required",
	"配置":                    "Configure",
	"无结果":                   "No Results",
	"返回":                    "Back",
	"应用":                    "Apply",
	"清除会话 Key":              "Clear Session Key",
	"仅在本次会话使用":              "Session only; not persisted to disk",
	"重试":                    "Retry",
	"上一页":                   "Previous Page",
	"下一页":                   "Next Page",
	"返回搜索":                  "Back to Search",
	"核对项目":                  "Match Project",
	"正在读取…":                 "Loading…",
	"下载量":                   "Downloads",
	"简介":                    "Overview",
	"版本":                    "Versions",
	"详情视图":                  "Project Tabs",
	"项目主页":                  "Website",
	"完整说明":                  "Full Page",
	"在浏览器中打开项目主页":           "Open project page in browser",
	"没有匹配版本":                "No Matching Versions",
	"相关度":                   "Relevance",
	"关注数":                   "Follows",
	"最新发布":                  "Newest",
	"最近更新":                  "Updated",
	"名称":                    "Name",
	"热门":                    "Popular",
	"加载器":                   "Loaders",
	"类别":                    "Categories",
	"环境":                    "Environments",
	"客户端":                   "Client",
	"服务端":                   "Server",
	"开源":                    "Open Source",
	"显示更多":                  "Show More",
	"读取中…":                  "Loading…",
	"类别不可用":                 "Categories unavailable",
	"重试类别":                  "Retry Categories",
	"无可用类别":                 "No Categories",
	"特性":                    "Features",
	"性能开销":                  "Performance Impact",
	"冒险":                    "Adventure",
	"科技":                    "Technology",
	"优化":                    "Optimization",
	"库":                     "Library",
	"实用":                    "Utility",
	"世界生成":                  "World Gen",
	"装饰":                    "Decoration",
	"装备":                    "Equipment",
	"魔法":                    "Magic",
	"管理":                    "Management",
	"生物":                    "Mobs",
	"存储":                    "Storage",
	"交通":                    "Transportation",
	"食物":                    "Food",
	"游戏机制":                  "Game Mechanics",
	"社交":                    "Social",
	"写实":                    "Realistic",
	"奇幻":                    "Fantasy",
	"原版":                    "Vanilla",
	"返回版本":                  "Back to Versions",
	"文件已安装":                 "Files Installed",
	"查看实例":                  "View Instance",
	"正在解析依赖…":               "Resolving dependencies…",
	"正在核验本地文件…":             "Verifying local files…",
	"正在检查并写入实例…":            "Checking and writing files…",

	// Servers
	"全部服务器":                    "All Servers",
	"在线服务器":                    "Online Servers",
	"全局添加":                     "Global",
	"按实例筛选":                    "By Instance",
	"重新收录实例服务器":                "Rescan Instance Servers",
	"添加服务器":                    "Add Server",
	"搜索服务器":                    "Search Servers",
	"搜索名称或地址":                  "Search name or address",
	"刷新服务器状态":                  "Refresh Server Status",
	"请输入服务器地址":                 "Please enter a server address",
	"已添加到全局服务器列表":              "Added to global server list",
	"关闭":                       "Close",
	"输入服务器地址，如 mc.example.com": "Enter server address, e.g. mc.example.com",
	"默认启动实例":                   "Default Instance",
	"自动选择":                     "Auto",
	"取消":                       "Cancel",
	"暂无已收录的服务器":                "No Servers Found",
	"在任意实例游戏内添加服务器，或点击右上角添加全局服务器。": "Add servers inside any Minecraft instance or click Add Server above.",
	"没有匹配的服务器":          "No Matching Servers",
	"可以调整筛选条件或清除搜索关键词。": "Try adjusting filters or clearing the search query.",
	"收录实例服务器":           "Scan Instance Servers",
	"全局":                "Global",
	"复制服务器地址":           "Copy Address",
	"探测中":               "Pinging",
	"未探测":               "Pending",
	"刷新状态":              "Refresh Status",
	"从启动器移除":            "Remove from Launcher",
	"正在探测服务器信息与 MOTD…":  "Querying server status & MOTD…",
	"点击右上角刷新可探测服务器 MOTD 与在线状态": "Click refresh to query server MOTD & player count",
	"无可用实例":      "No Instance",
	"选择启动实例":     "Select Instance",
	"加入":         "Join",
	"请先创建一个游戏实例": "Please create an instance first",
	"当前已有启动或安装任务正在进行": "Another launch or install task is in progress",
	"暂不支持该实例的加载器":     "This instance's loader is not yet supported",
	"请先选择或添加游戏账户":     "Please select or add a player account",

	// Credits & About
	"启动器的开发和发展离不开他们的辛苦付出。": "Thank you to the core contributors behind Meow Craft Launcher.",
	"自动检查":         "Automatic Checks",
	"每天在后台检查一次新版本": "Check for updates daily in the background",
	"自动检查更新":       "Check Automatically",
	"自动安装":         "Automatic Downloads",
	"下载后在下次启动时生效":  "Install downloaded updates on next restart",
	"自动下载并安装更新":    "Download and Install Updates",
	"上次检查":         "Last Check",
	"尚未检查":         "Never Checked",
	"当前构建不能自动更新：开发版本，或所在文件夹不可写。请从发布页下载新版本。": "Automatic updates are unavailable for development or read-only builds. Download releases from GitHub.",
	"已是最新版本": "Up to date",
	"正在检查…":  "Checking…",
	"下载更新":   "Download Update",
	"重启并安装":  "Restart & Install",
	"检查更新":   "Check for Updates",

	// Instances, Creation, Launch & Tasks
	"正在处理…":        "Working…",
	"实例设置":         "Instance Settings",
	"尚未选择实例":       "No instance selected",
	"返回实例":         "Back to Instance",
	"重新读取并放弃未保存修改": "Reload and discard unsaved changes",
	"正在读取实例…":      "Loading instance…",
	"基本信息":         "General",
	"图标":           "Icon",
	"更换实例图标":       "Change Icon",
	"更换图标":         "Change Icon",
	"目录未登记":        "Unregistered Directory",
	"复制路径":         "Copy Path",
	"打开游戏目录":       "Open Game Folder",
	"迁移…":          "Migrate…",
	"描述":           "Description",
	"运行环境":         "Runtime",
	"Java 偏好":      "Java Preference",
	"实例 Java 路径":   "Instance Java Path",
	"选择实例 Java":    "Select Instance Java",
	"选择 Java":      "Select Java",
	"删除实例及其全部数据":   "Delete Instance & All Data",
	"删除实例…":        "Delete Instance…",
	"实例图标":         "Instance Icon",
	"全部":           "All",
	"搜索实例资产":       "Search Assets",
	"资产排序":         "Sort Assets",
	"修改日期":         "Modified",
	"文件大小":         "Size",
	"刷新资产":         "Refresh Assets",
	"添加资产":         "Add Assets",
	"检查整合包文件":      "Verify Modpack Files",
	"撤销移除":         "Undo Removal",
	"恢复上次移除的资产，已有同名文件会保留": "Restore previously removed asset (keeps existing conflicts)",
	"打开实例目录":      "Open Instance Folder",
	"查看日志":        "View Logs",
	"新建":          "Create",
	"导入":          "Import",
	"新实例名称":       "New Instance Name",
	"实例名称":        "Instance Name",
	"游戏版本":        "Game Version",
	"选择版本":        "Select Version",
	"加载器版本":       "Loader Version",
	"暂无兼容版本":      "No Compatible Versions",
	"刷新加载器版本":     "Refresh Loader Versions",
	"创建后安装":       "Install after creation",
	"创建实例":        "Create",
	"创建并安装":       "Create & Install",
	"请选择列表中的正式版本": "Please select a release version",
	"请选择可用的游戏目录":  "Please select an available game directory",
	"请选择可用的加载器版本": "Please select a loader version",
	"已取消创建":       "Creation canceled",
	"实例已创建，游戏目录与其他实例隔离。": "Instance created with an isolated game directory.",
	"没有可启动的版本":           "No Instance",
	"先创建一个实例":            "Create an instance first",
	"等待游戏窗口":             "Waiting for Window",
	"游戏正在运行":             "Running",
	"游戏仍在运行":             "Still Running",
	"查看启动任务":             "View Launch",
	"查看安装任务":             "View Install",
	"识别资产中":              "Identifying Assets",
	"核对资产来源中":            "Matching Assets",
	"正在检查资产":             "Checking Assets",
	"修改资产中":              "Updating Assets",
	"正在恢复整合包文件":          "Restoring Pack Files",
	"查看资产更新":             "View Asset Update",
	"请先完成或恢复资产更新":        "Finish or recover pending update",
	"暂不支持此加载器":           "Unsupported Loader",
	"选择账户":               "Sign In",
	"添加微软账户或离线 ID":       "Add Microsoft or Offline Account",
	"检查启动条件":             "Prepare Launch",
	"安装游戏文件":             "Install Game",
	"等待游戏窗口…":            "Waiting for game window…",
	"正在取消准备…":            "Canceling preparation…",
	"游戏已停止":              "Game stopped",
	"游戏异常退出":             "Game crashed or exited unexpectedly",
	"游戏在窗口出现前退出":         "Game exited before opening a window",
	"游戏已退出":              "Game exited",
	"进程窗口已出现":            "Game window open",
	"游戏进程运行中":            "Game process running",
	"启动未完成":              "Launch incomplete",
	"打开终端日志":             "Open Logs",
	"停止游戏":               "Stop Game",
	"停止启动准备":             "Stop Launch Preparation",
	"返回任务列表":             "Back to Tasks",
	"会话与其他任务":            "Sessions & Other Tasks",
	"刷新任务":               "Refresh Tasks",
	"任务维护":               "Maintenance",
	"清理已结束记录…":           "Clear Finished Records…",
	"清理下载分片…":            "Clear Partial Downloads…",
	"重新读取文件状态":           "Reload File Status",
	"暂停":                 "Pause",
	"继续":                 "Resume",
	"继续准备":               "Resume Preparation",
	"复制诊断":               "Copy Diagnostics",
	"等待":                 "Pending",
	"检查缓存":               "Checking Cache",
	"校验":                 "Verifying",
	"发布":                 "Publishing",
	"已校验":                "Verified",
	"已跳过":                "Skipped",

	// Missing & newly wired UI / helper strings
	" 更新内容":                                   " Release Notes",
	"%d 个路径无法读取或运行":                           "%d paths could not be read or run",
	"%d / %d 个文件 · %.1f / %.1f MiB · 复用 %d 个": "%d / %d files · %.1f / %.1f MiB · %d reused",
	"%s / %s · %d / %d 文件":                    "%s / %s · %d / %d files",
	"%s 的账户选项":                                "Account options for %s",
	"Java %s 的操作":                             "Actions for Java %s",
	"下载任务":                                    "Download Tasks",
	"任务日志":                                    "Task Logs",
	"关联市场来源":                                  "Link Market Source",
	"切换到 %s":                                  "Switch to %s",
	"刷新会话":                                    "Refresh Sessions",
	"刷新版本":                                    "Refresh Versions",
	"加载器 ":                                    "Loader ",
	"发现整合包":                                   "Discover Modpacks",
	"启动状态":                                    "Launch Status",
	"启用 ":                                     "Enable ",
	"图标 ":                                     "Icon ",
	"安装进度":                                    "Installation Progress",
	"排序：":                                     "Sort: ",
	"推荐":                                      "Recommended",
	"搜索加载器版本":                                 "Search loader versions",
	"搜索版本":                                    "Search versions",
	"暂无任务":                                    "No active tasks",
	"查看任务":                                    "View Tasks",
	"查看启动":                                    "View Launch",
	"查看实例设置":                                  "View Instance Settings",
	"正在读取整合包…":                                "Reading modpack…",
	"没有匹配的版本":                                 "No matching versions",
	"游戏仍在运行 · Minecraft %s · PID %d": "Game still running · Minecraft %s · PID %d",
	"版本 ":        "Version ",
	"目录迁移与复制任务…": "Directory Move & Copy Tasks…",
	"移除 ":        "Remove ",
	"移除任务记录":     "Remove Task Record",
	"移除，可撤销；文件保留在实例的 removed-assets 目录": "Remove (undoable; kept in removed-assets)",
	"请在游戏窗口中退出。上次启动器的日志无法在此恢复。":         "Quit from inside the game window. Logs from the previous launcher session cannot be restored here.",
	"账户 · ":    "Account · ",
	"账户 · 未登录": "Account · Not Signed In",
	"进度按完整校验的文件计数。取消后可重新安装，完整文件会直接复用。": "Progress tracks verified files. Completed files are reused if canceled and restarted.",
	"退出代码 %d":            "Exit code %d",
	"退出启动页，返回实例":         "Leave Launch View",
	"选择图片…":              "Choose Image…",
	"选择实例图标":             "Choose Instance Icon",
	"选择版本 ":              "Select version ",
	"重新启动":               "Relaunch",
	"预发布":                "Pre-release",
	"从 Modrinth 下载整合包":   "Download Modrinth Modpack",
	"从 CurseForge 下载整合包": "Download CurseForge Modpack",
	"其他启动器占用":            "Locked by Another Launcher",
	"等待中":                "Queued",
	"正在暂停…":              "Pausing…",
	"可继续":                "Resumable",
	"未完成":                "Incomplete",
	"已完成":                "Completed",
	"等待安装":               "Queued for installation",
	"准备玩家账户":             "Preparing player account",
	"启动游戏进程":             "Starting game process",
	"检查 Java 版本与架构":      "Checking Java version and architecture",
	"寻找合适的 Java":         "Finding compatible Java",
	"获取 Java 运行环境":       "Fetching Java runtime catalog",
	"自动下载 Java":          "Downloading Java automatically",
	"安装并检查 Java":         "Installing and verifying Java",
	"查找安装工具 Java":        "Finding installer Java",
	"获取安装工具 Java":        "Fetching installer Java catalog",
	"下载安装工具 Java":        "Downloading installer Java",
	"安装并检查工具 Java":       "Installing and verifying installer Java",
	"读取版本信息":             "Reading version metadata",
	"配置模组加载器":            "Configuring mod loader",
	"下载加载器与处理器依赖":        "Downloading loader & processor dependencies",
	"生成加载器客户端文件":         "Generating loader client files",
	"下载与校验游戏文件":          "Downloading & verifying game files",
	"下载与校验资产对象":          "Downloading & verifying game assets",
	"准备历史资源":             "Preparing legacy assets",
	"准备本地库":              "Preparing native libraries",
	"准备任务":               "Preparing task",
	"读取元数据":              "Reading metadata",
	"解析 Java":            "Resolving Java",
	"执行安装处理器":            "Running install processors",
	"等待网络":               "Waiting for network",
	"等待下载名额":             "Waiting for download slot",
	"等待安装名额":             "Waiting for install slot",
	"按设置限速":              "Rate limited",
	"等待共享目标":             "Waiting for shared target",
	"等待重试":               "Waiting to retry",
	"准备文件":               "Preparing files",
	"Vanilla · 草方块":      "Vanilla · Grass Block",
	"Fabric · 布卷":        "Fabric · Fabric Roll",
	"Forge · 铁砧":         "Forge · Anvil",
	"NeoForge · 狐狸":      "NeoForge · Fox",
	"Quilt · 拼布":         "Quilt · Quilt",
	"mcl · 猫咪":           "mcl · Cat",
	"树叶":                 "Leaves",
	"星光":                 "Starlight",
	"工作台":                "Crafting Table",
}

var jaTranslations = map[string]string{
	// Navigation & Titlebar
	"主导航":     "メインナビゲーション",
	"副侧栏":     "サブサイドバー",
	"设置导航":    "設定ナビゲーション",
	"主页":      "ホーム",
	"实例":      "インスタンス",
	"市场":      "マーケット",
	"服务器":     "サーバー",
	"设置":      "設定",
	"收起或展开侧栏": "サイドバー切り替え",
	"游戏库":     "ライブラリ",
	"下载与任务":   "ダウンロードとタスク",
	"切换明暗主题":  "外観テーマ切り替え",
	"最小化窗口":   "最小化",
	"最大化窗口":   "最大化",
	"还原窗口":    "元のサイズに戻す",
	"关闭窗口":    "ウィンドウを閉じる",
	"暂无实例":    "インスタンスなし",
	"搜索实例":    "インスタンスを検索",
	"新建实例":    "新規インスタンス",

	// Settings Sidebar
	"游戏设置":    "ゲーム設定",
	"个性化":     "個人用設定",
	"国际化":     "言語・地域設定",
	"下载设置":    "ダウンロード設定",
	"Java 管理": "Java 管理",
	"账户":      "アカウント",
	"关于与更新":   "バージョン情報と更新",
	"鸣谢":      "クレジット",

	// Internationalization Page
	"语言":            "言語",
	"语言 (Language)": "言語 (Language)",
	"自动设置实例语言":      "インスタンス言語を自動設定",
	"启用后，新建实例的语言将自动设置为启动器语言，这将同时跳过游戏内辅助功能设置": "有効にすると、新規作成・起動時のインスタンス言語をランチャーに合わせ、初回アクセシビリティ案内をスキップします",
	"显示资源翻译": "リソース翻訳を表示",
	"启用后，在资源下载界面尝试显示名称或描述的简体中文翻译（如数据可用）": "有効にすると、利用可能な場合にリソース名や説明の翻訳を表示します",
	"资源文件名添加翻译前缀": "ファイル名に翻訳プレフィックスを追加",
	"启用后，下载与更新资源时将在文件名添加简体中文译名前缀（如“[地图] map.jar”）": "有効にすると、ダウンロードや更新時にファイル名へ翻訳タグを追加します（例：「[地图] map.jar」）",

	// Home View
	"个性化设置":    "個人用設定",
	"随机换一张墙纸":  "ランダム壁紙に切り替え",
	"未选择角色":    "アカウント未選択",
	"点击添加账户":   "クリックして追加",
	"离线模式":     "オフライン",
	"正版账户":     "Microsoft アカウント",
	"打开账户管理":   "アカウント管理を開く",
	"切换账户":     "アカウント切り替え",
	"离线":       "オフライン",
	"微软":       "Microsoft",
	"账户管理…":    "アカウント管理…",
	"启动游戏":     "ゲーム起動",
	"尚未选择游戏实例": "インスタンス未選択",
	"选择游戏实例":   "インスタンス選択",
	"切换或新建实例":  "切り替え・新規作成",
	"新建实例…":    "新規インスタンス…",
	"管理全部实例…":  "すべてのインスタンス…",

	// Personalization Page
	"我的墙纸": "マイ壁紙",
	"收起":   "折りたたむ",
	"添加墙纸": "壁紙を追加",
	"从本地选择一张或多张图片作为墙纸": "ローカルから1枚以上の画像を選択します",
	"混合随机切换": "シャッフル切り替え",
	"每次启动随机切换，点击立即换一张": "ランダムに壁紙を切り替えます（クリックで変更）",
	"mcl 默认壁纸":   "mcl デフォルト壁紙",
	"mcl 动漫默认壁纸": "mcl デフォルト壁紙",
	"默认":         "デフォルト",
	"设为墙纸":       "壁紙に設定",
	"移除墙纸":       "壁紙を削除",
	"背景效果":       "背景効果",
	"恢复默认":       "リセット",
	"恢复默认效果":     "デフォルトに戻す",
	"换一张":        "次へ",
	"背景不透明度":     "不透明度",
	"背景模糊":       "ぼかし",
	"背景压暗":       "暗さ調整",
	"已添加墙纸":      "壁紙を追加しました",
	"已移除墙纸":      "壁紙を削除しました",

	// Game & Download Settings
	"分辨率":        "解像度",
	"自定义":        "カスタム",
	"全屏启动":       "フルスクリーン",
	"自定义信息":      "カスタム情報",
	"自动加入服务器":    "サーバー自動接続",
	"服务器地址":      "サーバーアドレス",
	"游戏内存":       "メモリ割り当て",
	"自动分配":       "自動割り当て",
	"进程优先级":      "プロセス優先度",
	"启动后显示日志":    "起動後にログを表示",
	"内存管理":       "GC 方式",
	"JVM 参数":     "JVM 引数",
	"自定义 JVM 参数": "カスタム JVM 引数",
	"无":          "なし",
	"尽量使用 ZGC":   "ZGC を優先",
	"尽量使用分代 ZGC": "世代別 ZGC を優先",
	"标准 G1GC":    "標準 G1GC",
	"调优 G1GC":    "チューニング済み G1GC",
	"不指定（可自定义）":  "指定しない（カスタム可）",
	"低":          "低",
	"低于正常":       "通常以下",
	"正常":         "通常",
	"高于正常":       "通常以上",
	"高":          "高",
	"游戏目录":       "ゲームディレクトリ",
	"下载源":        "ダウンロード元",
	"并发下载":       "同時ダウンロード数",
	"速度限制":       "速度制限",
	"下载代理":       "プロキシ",
	"代理地址":       "プロキシアドレス",
	"仅官方":        "公式のみ",
	"自动":         "自動",
	"官方优先":       "公式優先",
	"镜像优先":       "ミラー優先",
	"环境变量":       "環境変数",
	"直连":         "直接接続",
	"不限速":        "無制限",
	"未知":         "不明",

	// Java & Accounts
	"下载":         "ダウンロード",
	"添加":         "追加",
	"安装":         "インストール",
	"已安装":        "インストール済み",
	"已取消":        "キャンセル済み",
	"已暂停":        "一時停止中",
	"正在取消…":      "キャンセル中…",
	"查看":         "表示",
	"设为全局默认":     "デフォルトに設定",
	"打开所在文件夹":    "フォルダを開く",
	"复制 Java 路径": "パスをコピー",
	"从列表移除":      "リストから削除",
	"添加账户":       "アカウント追加",
	"微软账户":       "Microsoft アカウント",
	"离线 ID":      "オフライン ID",
	"还没有账户":      "アカウントがありません",
	"登录微软账户":     "Microsoft でサインイン",
	"添加离线 ID":    "オフライン ID を追加",
	"取消登录":       "キャンセル",
	"移除账户":       "アカウント削除",

	// Market & Servers
	"模组":     "MOD",
	"光影":     "シェーダー",
	"资源包":    "リソースパック",
	"数据包":    "データパック",
	"整合包":    "MODパック",
	"地图":     "ワールド",
	"不指定实例":  "インスタンス指定なし",
	"搜索":     "検索",
	"全部版本":   "すべてのバージョン",
	"无结果":    "結果なし",
	"返回":     "戻る",
	"应用":     "適用",
	"重试":     "再試行",
	"简介":     "概要",
	"版本":     "バージョン",
	"项目主页":   "プロジェクトページ",
	"完整说明":   "詳細ページ",
	"没有匹配版本": "一致するバージョンがありません",
	"相关度":    "関連度",
	"下载量":    "ダウンロード数",
	"关注数":    "フォロー数",
	"最新发布":   "新着順",
	"最近更新":   "更新順",
	"名称":     "名前",
	"热门":     "人気",
	"加载器":    "ローダー",
	"类别":     "カテゴリー",
	"环境":     "実行環境",
	"客户端":    "クライアント",
	"服务端":    "サーバー",
	"开源":     "オープンソース",
	"显示更多":   "もっと見る",
	"原版":     "バニラ",
	"全部服务器":  "すべてのサーバー",
	"在线服务器":  "オンライン",
	"全局添加":   "グローバル追加",
	"按实例筛选":  "インスタンス別",
	"添加服务器":  "サーバーを追加",
	"取消":     "キャンセル",
	"自动选择":   "自動選択",
	"全局":     "グローバル",
	"探测中":    "取得中",
	"未探测":    "未取得",
	"加入":     "参加",
	"关闭":     "閉じる",

	// Credits, Updates, Launch & Tasks
	"启动器的开发和发展离不开他们的辛苦付出。": "開発を支えるコアコントリビューターの皆様に感謝します。",
	"自动检查":         "自動確認",
	"每天在后台检查一次新版本": "1日に1回バックグラウンドで更新を確認します",
	"自动安装":         "自動ダウンロード",
	"下载后在下次启动时生效":  "ダウンロード後、次回起動時に適用されます",
	"上次检查":         "最終確認",
	"尚未检查":         "未確認",
	"已是最新版本":       "最新バージョンです",
	"正在检查…":        "確認中…",
	"下载更新":         "更新をダウンロード",
	"重启并安装":        "再起動してインストール",
	"检查更新":         "アップデートを確認",
	"新建":           "新規作成",
	"导入":           "インポート",
	"全部":           "すべて",
	"停止游戏":         "ゲーム停止",
	"停止启动准备":       "起動準備を中止",
	"打开终端日志":       "ログを開く",
	"等待游戏窗口…":      "ゲームウィンドウ待機中…",
	"正在取消准备…":      "中止処理中…",
	"游戏已停止":        "ゲームを停止しました",
	"游戏异常退出":       "ゲームが異常終了しました",
	"游戏在窗口出现前退出":   "ウィンドウ表示前に終了しました",
	"游戏已退出":        "ゲームが終了しました",
	"进程窗口已出现":      "ウィンドウ表示完了",
	"游戏进程运行中":      "ゲーム実行中",
	"没有可启动的版本":     "インスタンスなし",
	"先创建一个实例":      "先にインスタンスを作成してください",
	"等待游戏窗口":       "ウィンドウ待機中",
	"游戏正在运行":       "実行中",
	"游戏仍在运行":       "実行中",
	"查看启动任务":       "起動状況を表示",
	"查看安装任务":       "インストール状況を表示",
	"安装游戏文件":       "インストール",
	"检查启动条件":       "起動チェック",
	"选择账户":         "アカウント選択",
	"暂停":           "一時停止",
	"继续":           "再開",

	// Missing & newly wired UI / helper strings
	" 更新内容":                                   " リリースノート",
	"%d 个路径无法读取或运行":                           "%d 個のパスを読み取れないか実行できません",
	"%d / %d 个文件 · %.1f / %.1f MiB · 复用 %d 个": "%d / %d ファイル · %.1f / %.1f MiB · %d 再利用",
	"%s / %s · %d / %d 文件":                    "%s / %s · %d / %d ファイル",
	"%s 的账户选项":                                "%s のアカウント設定",
	"Java %s 的操作":                             "Java %s の操作",
	"Java 主版本":                                "Java メジャーバージョン",
	"Java 偏好":                                 "Java 設定",
	"上一页":                                     "前のページ",
	"下一页":                                     "次のページ",
	"下载 Java":                                 "Java をダウンロード",
	"下载任务":                                    "ダウンロードタスク",
	"主机":                                      "ホスト",
	"仅在本次会话使用":                                "このセッションのみ有効",
	"任务日志":                                    "タスクログ",
	"从启动器移除":                                  "ランチャーから削除",
	"从本地皮肤设置头像":                               "ローカルスキンからアイコン設定",
	"代理主机":                                    "プロキシホスト",
	"代理端口":                                    "プロキシポート",
	"任务维护":                                    "タスク管理",
	"会话与其他任务":                                 "セッションとその他のタスク",
	"使用同名正版头像":                                "同名正規アカウントのアバターを使用",
	"修改日期":                                    "更新日時",
	"全局默认 Java":                               "グローバル既定 Java",
	"关联市场来源":                                  "マーケットソースを関連付け",
	"关闭 Java 任务":                              "Java タスクを閉じる",
	"关闭离线账户表单":                                "オフラインアカウント入力を閉じる",
	"切换到 %s":                                  "%s に切り替え",
	"创建后安装":                                   "作成後にインストール",
	"删除实例…":                                   "インスタンスを削除…",
	"删除实例及其全部数据":                              "インスタンスとすべてのデータを削除",
	"刷新下载目录":                                  "カタログを更新",
	"刷新任务":                                    "タスクを更新",
	"刷新会话":                                    "セッションを更新",
	"刷新加载器版本":                                 "ローダーバージョンを更新",
	"刷新服务器状态":                                 "サーバー状態を更新",
	"刷新版本":                                    "バージョンを更新",
	"刷新状态":                                    "状態を更新",
	"刷新资产":                                    "アセットを更新",
	"加载器 ":                                    "ローダー ",
	"加载器版本":                                   "ローダーバージョン",
	"发现整合包":                                   "Modpack を探す",
	"取消安装 Java":                               "Java インストールをキャンセル",
	"启动后显示游戏日志":                               "起動後にログを表示",
	"启动状态":                                    "起動ステータス",
	"启用 ":                                     "有効化 ",
	"图标":                                      "アイコン",
	"图标 ":                                     "アイコン ",
	"安装进度":                                    "インストール進捗",
	"在浏览器中打开项目主页":                             "ブラウザでプロジェクトページを開く",
	"基本信息":                                    "基本情報",
	"复制服务器地址":                                 "サーバーアドレスをコピー",
	"复制诊断":                                    "診断情報をコピー",
	"复制路径":                                    "パスをコピー",
	"实例 Java 路径":                              "インスタンスの Java パス",
	"实例名称":                                    "インスタンス名",
	"实例图标":                                    "インスタンスアイコン",
	"实例设置":                                    "インスタンス設定",
	"尚未发现 Java":                               "Java が見つかりません",
	"市场排序":                                    "並び替え",
	"当前构建不能自动更新：开发版本，或所在文件夹不可写。请从发布页下载新版本。": "このビルドは自動更新できません（開発版または書き込み不可の場所）。リリースページからダウンロードしてください。",
	"当前账户": "現在のアカウント",
	"微软登录需要配置 mcl 自有的应用 Client ID": "Microsoft ログインには mcl 独自のアプリ Client ID 設定が必要です",
	"恢复上次移除的资产，已有同名文件会保留":          "前回削除したアセットを復元（同名ファイルは保持）",
	"打开实例目录":           "インスタンスフォルダを開く",
	"打开游戏目录":           "ゲームフォルダを開く",
	"排序：":              "並び替え：",
	"推荐":               "推奨",
	"描述":               "説明",
	"搜索加载器版本":          "ローダーバージョンを検索",
	"搜索名称或地址":          "名前またはアドレスを検索",
	"搜索实例资产":           "インスタンスアセットを検索",
	"搜索市场":             "マーケットを検索",
	"搜索服务器":            "サーバーを検索",
	"搜索版本":             "バージョンを検索",
	"撤销移除":             "削除を取り消す",
	"收录实例服务器":          "インスタンスのサーバーを取り込む",
	"文件大小":             "ファイルサイズ",
	"文件已安装":            "インストール済み",
	"新实例名称":            "新規インスタンス名",
	"无可用实例":            "利用可能なインスタンスがありません",
	"无可用类别":            "利用可能なカテゴリがありません",
	"暂无任务":             "アクティブなタスクはありません",
	"更多操作":             "詳細操作",
	"更换图标":             "アイコンを変更",
	"更换实例图标":           "インスタンスアイコンを変更",
	"查看任务":             "タスクを表示",
	"查看启动":             "起動を表示",
	"查看实例":             "インスタンスを表示",
	"查看实例设置":           "インスタンス設定を表示",
	"查看日志":             "ログを表示",
	"核对项目":             "プロジェクトを照合",
	"检查整合包文件":          "Modpack ファイルを確認",
	"正在处理…":            "処理中…",
	"正在探测服务器信息与 MOTD…": "サーバー情報と MOTD を取得中…",
	"正在核验本地文件…":        "ローカルファイルを検証中…",
	"正在检查并写入实例…":       "確認してインスタンスへ書き込み中…",
	"正在添加离线账户":         "オフラインアカウントを追加中",
	"正在解析依赖…":          "依存関係を解析中…",
	"正在读取…":            "読み込み中…",
	"正在读取实例…":          "インスタンスを読み込み中…",
	"正在读取整合包…":         "Modpack を読み込み中…",
	"正在读取账户":           "アカウントを読み込み中",
	"此版本暂无适合本机的发行包":    "この環境に対応する配布パッケージはありません",
	"没有匹配的版本":          "一致するバージョンがありません",
	"游戏仍在运行 · Minecraft %s · PID %d": "ゲーム実行中 · Minecraft %s · PID %d",
	"添加本机 Java":                      "ローカル Java を追加",
	"添加资产":                           "アセットを追加",
	"清理下载分片…":                        "一時ファイルを削除…",
	"清理已结束记录…":                       "完了済み履歴を削除…",
	"清除会话 Key":                       "セッションキーを消去",
	"清除筛选":                           "絞り込みを解除",
	"游戏分辨率":                          "ゲーム解像度",
	"游戏版本":                           "ゲームバージョン",
	"游戏窗口宽度":                         "ウィンドウ幅",
	"游戏窗口高度":                         "ウィンドウ高さ",
	"点击右上角刷新可探测服务器 MOTD 与在线状态": "右上の更新ボタンでサーバーの MOTD と状態を確認できます",
	"版本 ":        "バージョン ",
	"玩家名称":       "プレイヤー名",
	"目录迁移与复制任务…": "フォルダ移行・コピータスク…",
	"目录未登记":      "フォルダ未登録",
	"离线玩家名称":     "オフラインプレイヤー名",
	"移除 ":        "削除 ",
	"移除任务记录":     "タスク履歴を削除",
	"移除，可撤销；文件保留在实例的 removed-assets 目录": "削除（取り消し可、removed-assets に退避）",
	"端口":              "ポート",
	"筛选游戏版本":          "ゲームバージョンで絞り込む",
	"类别不可用":           "カテゴリ利用不可",
	"自动下载并安装更新":       "更新の自動ダウンロードとインストール",
	"自动检查更新":          "更新の自動確認",
	"自定义下载并发数":        "最大同時ダウンロード数",
	"自定义下载速率上限，KiB/s": "ダウンロード速度上限 (KiB/s)",
	"自定义游戏信息":         "カスタムゲーム情報",
	"请在游戏窗口中退出。上次启动器的日志无法在此恢复。": "ゲーム画面から終了してください。前回の起動セッションのログはここで復元できません。",
	"详情视图":         "詳細ビュー",
	"读取 Java 官方目录": "Java 公式カタログを読み込み中",
	"读取中…":         "読み込み中…",
	"账户 · ":        "アカウント · ",
	"账户 · 未登录":     "アカウント · 未ログイン",
	"账户选项":         "アカウント設定",
	"资产排序":         "アセット並び替え",
	"输入服务器地址，如 mc.example.com":         "サーバーアドレスを入力 (例: mc.example.com)",
	"进度按完整校验的文件计数。取消后可重新安装，完整文件会直接复用。": "検証済みファイル数で進捗を表示します。キャンセル後、完了済みファイルは再利用されます。",
	"迁移…":                   "移行…",
	"运行环境":                  "実行環境",
	"返回 Java 管理":            "Java 管理に戻る",
	"返回任务列表":                "タスク一覧に戻る",
	"返回实例":                  "インスタンスに戻る",
	"返回搜索":                  "検索に戻る",
	"返回版本":                  "バージョンに戻る",
	"退出代码 %d":               "終了コード %d",
	"退出启动页，返回实例":            "起動画面を閉じてインスタンスへ戻る",
	"选择 Java":               "Java を選択",
	"选择 Java 目录":            "Java フォルダを選択",
	"选择可执行文件":               "実行ファイルを選択",
	"选择启动实例":                "起動インスタンスを選択",
	"选择图片…":                 "画像を選択…",
	"选择实例 Java":             "インスタンス Java を選択",
	"选择实例图标":                "インスタンスアイコンを選択",
	"选择游戏版本":                "ゲームバージョンを選択",
	"选择加载器版本":               "ローダーバージョンを選択",
	"选择运行环境":                "実行環境を選択",
	"选择版本":                  "バージョンを選択",
	"选择版本 ":                 "バージョン選択 ",
	"选择目标实例":                "対象インスタンスを選択",
	"配置":                    "設定",
	"配置 CurseForge API Key": "CurseForge API Key を設定",
	"重新发现本机 Java":           "ローカル Java を再スキャン",
	"重新启动":                  "再起動",
	"重新收录实例服务器":             "サーバー一覧を再取り込み",
	"重新登录":                  "再ログイン",
	"重新读取并放弃未保存修改":          "再読み込みして未保存の変更を破棄",
	"重新读取文件状态":              "ファイル状態を再読み込み",
	"重新读取账户":                "アカウントを再読み込み",
	"重试类别":                  "カテゴリ再試行",
	"需要 CurseForge API Key": "CurseForge API Key が必要です",
	"预发布":                   "プレリリース",
	"默认启动实例":                "既定の起動インスタンス",
	"从 Modrinth 下载整合包":      "Modrinth から Modpack をダウンロード",
	"从 CurseForge 下载整合包":    "CurseForge から Modpack をダウンロード",
	"等待":                    "待機中",
	"检查缓存":                  "キャッシュ確認中",
	"校验":                    "検証中",
	"发布":                    "適用中",
	"已校验":                   "検証済み",
	"已跳过":                   "スキップ済み",
	"其他启动器占用":               "他のランチャーが使用中",
	"等待中":                   "待機中",
	"正在暂停…":                 "一時停止中…",
	"可继续":                   "再開可能",
	"未完成":                   "未完了",
	"已完成":                   "完了",
	"等待安装":                  "インストール待機中",
	"准备玩家账户":                "プレイヤーアカウント準備中",
	"启动游戏进程":                "ゲームプロセス起動中",
	"检查 Java 版本与架构":         "Java バージョンとアーキテクチャ確認中",
	"寻找合适的 Java":            "適切な Java を検索中",
	"获取 Java 运行环境":          "Java ランタイム情報を取得中",
	"自动下载 Java":             "Java を自動ダウンロード中",
	"安装并检查 Java":            "Java をインストール・検証中",
	"查找安装工具 Java":           "インストーラー用 Java を検索中",
	"获取安装工具 Java":           "インストーラー用 Java を取得中",
	"下载安装工具 Java":           "インストーラー用 Java をダウンロード中",
	"安装并检查工具 Java":          "インストーラー用 Java をインストール・検証中",
	"读取版本信息":                "バージョン情報読み込み中",
	"配置模组加载器":               "Mod ローダー設定中",
	"下载加载器与处理器依赖":           "ローダー・依存ファイルを取得中",
	"生成加载器客户端文件":            "ローダークライアントを生成中",
	"下载与校验游戏文件":             "ゲームファイルをダウンロード・検証中",
	"下载与校验资产对象":             "アセットをダウンロード・検証中",
	"准备历史资源":                "旧バージョンのアセットを準備中",
	"准备本地库":                 "ネイティブライブラリを準備中",
	"准备任务":                  "タスク準備中",
	"读取元数据":                 "メタデータ読み込み中",
	"解析 Java":               "Java を解決中",
	"执行安装处理器":               "インストールプロセッサ実行中",
	"等待网络":                  "ネットワーク待機中",
	"等待下载名额":                "ダウンロード枠を待機中",
	"等待安装名额":                "インストール枠を待機中",
	"按设置限速":                 "設定に基づき速度制限中",
	"等待共享目标":                "共有対象を待機中",
	"等待重试":                  "リトライ待機中",
	"准备文件":                  "ファイル準備中",
	"Vanilla · 草方块":         "Vanilla · 草ブロック",
	"Fabric · 布卷":           "Fabric · 布ロール",
	"Forge · 铁砧":            "Forge · 金床",
	"NeoForge · 狐狸":         "NeoForge · キツネ",
	"Quilt · 拼布":            "Quilt · キルト",
	"mcl · 猫咪":              "mcl · ネコ",
	"树叶":                    "葉っぱ",
	"星光":                    "星明かり",
	"工作台":                   "作業台",
}

var zhHantTranslations = map[string]string{
	"语言 (Language)": "語言 (Language)",
	"默认":            "預設",
	"恢复默认":          "重設為預設",
	"恢复默认效果":        "重設預設效果",
	"自动设置实例语言":      "自動設定執行個體語言",
	"启用后，新建实例的语言将自动设置为启动器语言，这将同时跳过游戏内辅助功能设置": "啟用後，新建或啟動執行個體的語言將自動設為啟動器語言，並跳過遊戲內協助工具導覽設定",
	"显示资源翻译": "顯示資源翻譯",
	"启用后，在资源下载界面尝试显示名称或描述的简体中文翻译（如数据可用）": "啟用後，在資源下載介面嘗試顯示名稱或描述的中文翻譯（如有資料可用）",
	"资源文件名添加翻译前缀": "資源檔名加入翻譯前綴",
	"启用后，下载与更新资源时将在文件名添加简体中文译名前缀（如“[地图] map.jar”）": "啟用後，下載與更新資源時會在檔名前加入中文譯名前綴（如「[地图] map.jar」）",
	"游戏内存":        "遊戲記憶體",
	"内存管理":        "記憶體管理",
	"默认启动实例":      "預設啟動執行個體",
	"原版":          "原版",
	"微软":          "微軟",
	"正版账户":        "正版帳戶",
	"离线模式":        "離線模式",
	"离线":          "離線",
	"在浏览器中打开项目主页": "在瀏覽器開啟專案首頁",
	"自动检查":        "自動檢查",
	"自动安装":        "自動安裝",
	"检查更新":        "檢查更新",
}

var zhHansToHantRune = map[rune]rune{
	'个': '個', '丰': '豐', '临': '臨', '为': '為', '举': '舉', '么': '麼', '义': '義',
	'乌': '烏', '乐': '樂', '习': '習', '乡': '鄉', '书': '書', '买': '買', '乱': '亂',
	'争': '爭', '于': '於', '亏': '虧', '云': '雲', '亚': '亞', '产': '產', '亲': '親',
	'仅': '僅', '从': '從', '仓': '倉', '仪': '儀', '们': '們', '价': '價', '众': '眾',
	'优': '優', '会': '會', '传': '傳', '伤': '傷', '伦': '倫', '体': '體', '余': '餘',
	'侧': '側', '侦': '偵', '俩': '倆', '储': '儲', '儿': '兒', '两': '兩', '关': '關',
	'兴': '興', '养': '養', '内': '內', '冈': '岡', '册': '冊', '写': '寫', '军': '軍',
	'农': '農', '冲': '衝', '决': '決', '况': '況', '净': '淨', '准': '準', '凉': '涼',
	'几': '幾', '凤': '鳳', '凭': '憑', '击': '擊', '划': '劃', '则': '則', '刚': '剛',
	'创': '創', '删': '刪', '别': '別', '制': '製', '剂': '劑', '剑': '劍', '剧': '劇',
	'办': '辦', '务': '務', '动': '動', '劳': '勞', '势': '勢', '勋': '勳', '区': '區',
	'医': '醫', '华': '華', '协': '協', '单': '單', '卖': '賣', '占': '佔', '却': '卻',
	'卷': '捲', '厂': '廠', '历': '歷', '厉': '厲', '压': '壓', '厌': '厭', '参': '參',
	'双': '雙', '发': '發', '变': '變', '叙': '敘', '号': '號', '后': '後', '吗': '嗎',
	'听': '聽', '启': '啟', '周': '週', '响': '響', '问': '問', '团': '團', '园': '園',
	'围': '圍', '图': '圖', '国': '國', '圆': '圓', '场': '場', '块': '塊', '坚': '堅',
	'坛': '壇', '坏': '壞', '执': '執', '声': '聲', '处': '處', '备': '備', '复': '復',
	'够': '夠', '头': '頭', '夸': '誇', '奥': '奧', '妈': '媽', '孙': '孫', '学': '學',
	'宁': '寧', '宝': '寶', '实': '實', '审': '審', '宫': '宮', '宽': '寬', '宾': '賓',
	'对': '對', '寻': '尋', '导': '導', '寿': '壽', '将': '將', '专': '專', '尔': '爾',
	'属': '屬', '岁': '歲', '岂': '豈', '岗': '崗', '岛': '島', '帅': '帥', '师': '師',
	'帐': '帳', '带': '帶', '帧': '幀', '帮': '幫', '干': '幹', '并': '並', '广': '廣',
	'庄': '莊', '庆': '慶', '库': '庫', '应': '應', '开': '開', '异': '異', '弃': '棄',
	'张': '張', '强': '強', '弹': '彈', '归': '歸', '当': '當', '录': '錄', '态': '態',
	'总': '總', '恶': '惡', '愿': '願', '戏': '戲', '战': '戰', '户': '戶', '扩': '擴',
	'扫': '掃', '扬': '揚', '抢': '搶', '护': '護', '报': '報', '择': '擇', '损': '損',
	'换': '換', '据': '據', '携': '攜', '摄': '攝', '摆': '擺', '摇': '搖', '数': '數',
	'斗': '鬥', '断': '斷', '无': '無', '旧': '舊', '时': '時', '显': '顯', '暂': '暫',
	'术': '術', '机': '機', '杀': '殺', '杂': '雜', '权': '權', '条': '條', '来': '來',
	'杨': '楊', '极': '極', '构': '構', '标': '標', '档': '檔', '检': '檢', '横': '橫',
	'欢': '歡', '气': '氣', '汇': '匯', '汉': '漢', '没': '沒', '洁': '潔', '测': '測',
	'渐': '漸', '游': '遊', '滤': '濾', '满': '滿', '滚': '滾', '潜': '潛', '灯': '燈',
	'灵': '靈', '点': '點', '热': '熱', '烦': '煩', '爱': '愛', '独': '獨', '狭': '狹',
	'猫': '貓', '现': '現', '环': '環', '电': '電', '画': '畫', '盖': '蓋', '盘': '盤',
	'着': '著', '码': '碼', '确': '確', '种': '種', '称': '稱', '稳': '穩', '竞': '競',
	'笔': '筆', '简': '簡', '紧': '緊', '红': '紅', '约': '約', '级': '級', '线': '線',
	'组': '組', '细': '細', '终': '終', '经': '經', '结': '結', '给': '給', '统': '統',
	'继': '繼', '续': '續', '维': '維', '综': '綜', '绿': '綠', '编': '編', '缓': '緩',
	'网': '網', '罗': '羅', '联': '聯', '职': '職', '脉': '脈', '节': '節', '范': '範',
	'获': '獲', '营': '營', '蓝': '藍', '装': '裝', '见': '見', '观': '觀', '规': '規',
	'视': '視', '览': '覽', '觉': '覺', '计': '計', '订': '訂', '认': '認', '议': '議',
	'记': '記', '讲': '講', '许': '許', '论': '論', '设': '設', '访': '訪', '证': '證',
	'评': '評', '识': '識', '诉': '訴', '译': '譯', '试': '試', '话': '話', '该': '該',
	'详': '詳', '语': '語', '误': '誤', '说': '說', '请': '請', '读': '讀', '调': '調',
	'谈': '談', '谢': '謝', '贝': '貝', '负': '負', '贡': '貢', '财': '財', '责': '責',
	'购': '購', '贴': '貼', '贵': '貴', '费': '費', '资': '資', '赋': '賦', '质': '質',
	'赢': '贏', '车': '車', '转': '轉', '轮': '輪', '软': '軟', '轻': '輕', '载': '載',
	'辅': '輔', '输': '輸', '达': '達', '过': '過', '迈': '邁', '运': '運', '还': '還',
	'这': '這', '进': '進', '远': '遠', '连': '連', '适': '適', '选': '選', '递': '遞',
	'遗': '遺', '邻': '鄰', '针': '針', '钟': '鐘', '钢': '鋼', '钥': '鑰', '铁': '鐵',
	'银': '銀', '链': '鏈', '锁': '鎖', '错': '錯', '键': '鍵', '长': '長', '门': '門',
	'闪': '閃', '闭': '閉', '闲': '閒', '间': '間', '际': '際', '隐': '隱', '难': '難',
	'静': '靜', '页': '頁', '顶': '頂', '项': '項', '顺': '順', '须': '須', '领': '領',
	'题': '題', '颜': '顏', '额': '額', '风': '風', '飞': '飛', '饮': '飲', '饰': '飾',
	'驱': '驅', '验': '驗', '鲜': '鮮', '齐': '齊', '齿': '齒', '龙': '龍', '墙': '牆',
	'纸': '紙', '随': '隨', '径': '徑', '账': '帳', '签': '籤', '缀': '綴', '离': '離',
	'络': '絡', '状': '狀', '辑': '輯', '败': '敗', '毕': '畢', '补': '補',
}

func toTraditionalChinese(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if trad, ok := zhHansToHantRune[r]; ok {
			b.WriteRune(trad)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
