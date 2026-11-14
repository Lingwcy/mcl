export type Locale = "en" | "zh";

export const REPO_URL = "https://github.com/Lingwcy/mcl";
export const RELEASES_URL = `${REPO_URL}/releases`;
export const LATEST_TAG = "0.0.7";

export const SCREENSHOT_WIDTH = 1360;
export const SCREENSHOT_HEIGHT = 1114;

export interface ScreenshotItem {
  id: string;
  src: string;
  alt: string;
  caption: string;
}

export interface FeatureCell {
  icon: string;
  title: string;
  body: string;
}

export interface ShowcaseBullet {
  icon?: string;
  loader?: "grass" | "fabric" | "forge" | "neoforge" | "quilt";
  label: string;
}

export interface ShowcaseItem {
  title: string;
  body: string;
  image: string;
  alt: string;
  backdrop: string;
  bullets: ShowcaseBullet[];
}

export interface ShowcaseSection {
  id: string;
  title: string;
  body: string;
  items: ShowcaseItem[];
}

export interface DownloadAsset {
  id: string;
  platform: "macos" | "windows";
  name: string;
  meta: string;
  url: string;
}

export interface SiteCopy {
  lang: string;
  title: string;
  description: string;
  skip: string;
  brand: string;
  brandFull: string;
  homePath: string;
  docsPath: string;
  otherLocale: Locale;
  otherLocaleLabel: string;
  otherHomePath: string;
  railNavLabel: string;
  menuOpenLabel: string;
  menuCloseLabel: string;
  nav: Array<{
    id: string;
    label: string;
    icon: string;
  }>;
  hero: {
    prefix: string;
    suffix: string;
    body: string;
    downloadMacArm: string;
    downloadWinX64: string;
    downloadLabel: string;
    otherDownloadsLabel: string;
    allReleasesLabel: string;
    buildFromSourceLabel: string;
    docsLabel: string;
    starLabel: string;
    prevShot: string;
    nextShot: string;
    closeLightbox: string;
  };
  downloads: DownloadAsset[];
  screenshots: ScreenshotItem[];
  about: {
    id: string;
    title: string;
    body: string;
    items: FeatureCell[];
  };
  showcases: ShowcaseSection[];
  opensource: {
    id: string;
    title: string;
    body: string;
    starsLabel: string;
    forksLabel: string;
    licenseLabel: string;
    licenseValue: string;
    viewRepoLabel: string;
    issuesLabel: string;
    can: {
      label: string;
      items: string[];
    };
    wont: {
      label: string;
      items: string[];
    };
  };
  faq: {
    id: string;
    title: string;
    items: Array<{
      q: string;
      a: string;
    }>;
  };
  footer: {
    watermark: string;
    disclaimer: string;
    mitLine: string;
    docsLink: string;
    githubLink: string;
    releasesLink: string;
  };
  docs: {
    siteLabel: string;
    homeLink: string;
    browseMenu: string;
    searchPlaceholder: string;
    searchHint: string;
    emptySearch: string;
    onThisPage: string;
    previous: string;
    next: string;
    copy: string;
    copied: string;
    copyCode: string;
    copiedCode: string;
  };
}

export const copy: Record<Locale, SiteCopy> = {
  zh: {
    lang: "zh-CN",
    title: "mcl — Meow Craft Launcher | 轻量原生 Minecraft 启动器",
    description:
      "用 Go 与 MyGo 原生构建的轻量 Minecraft: Java Edition 启动器。双源资源市场、五大加载器、整合包导入与自动 Java 管理，基于 MIT 协议完全开源。",
    skip: "跳到正文",
    brand: "mcl",
    brandFull: "Meow Craft Launcher",
    homePath: "/zh/",
    docsPath: "/zh/docs/",
    otherLocale: "en",
    otherLocaleLabel: "English",
    otherHomePath: "/",
    railNavLabel: "页面导航",
    menuOpenLabel: "打开导航",
    menuCloseLabel: "关闭导航",
    nav: [
      { id: "about", label: "核心特性", icon: "layers" },
      { id: "market", label: "资源市场", icon: "package" },
      { id: "modpacks", label: "实例与整合包", icon: "puzzle" },
      { id: "runtime", label: "运行与安全", icon: "cpu" },
      { id: "opensource", label: "开源承诺", icon: "code" },
      { id: "faq", label: "常见问题", icon: "help" },
    ],
    hero: {
      prefix: "为实用而造的",
      suffix: "Minecraft 启动器",
      body: "轻量、原生的 Minecraft: Java Edition 启动器。Go 与 MyGo 原生界面开发。没有 Electron，没有 Tauri，没有臃肿的浏览器壳。",
      downloadMacArm: "下载 macOS (Apple Silicon)",
      downloadWinX64: "下载 Windows (x64)",
      downloadLabel: "下载体验",
      otherDownloadsLabel: "选择平台安装包",
      allReleasesLabel: "查看所有 Releases & SHA256SUMS",
      buildFromSourceLabel: "从源码编译指南",
      docsLabel: "阅读文档",
      starLabel: "Star",
      prevShot: "上一张截图",
      nextShot: "下一张截图",
      closeLightbox: "关闭预览",
    },
    downloads: [
      {
        id: "darwin-arm64",
        platform: "macos",
        name: "macOS (Apple Silicon)",
        meta: `v${LATEST_TAG} · 已签名并公证 .dmg`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-darwin-arm64.dmg`,
      },
      {
        id: "darwin-amd64",
        platform: "macos",
        name: "macOS (Intel x86_64)",
        meta: `v${LATEST_TAG} · 已签名并公证 .dmg`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-darwin-amd64.dmg`,
      },
      {
        id: "windows-amd64",
        platform: "windows",
        name: "Windows (x64 便携版)",
        meta: `v${LATEST_TAG} · 单文件 .exe`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-windows-amd64.exe`,
      },
      {
        id: "windows-arm64",
        platform: "windows",
        name: "Windows (ARM64 便携版)",
        meta: `v${LATEST_TAG} · 单文件 .exe`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-windows-arm64.exe`,
      },
    ],
    screenshots: [
      {
        id: "home",
        src: "/screenshots/home.webp",
        alt: "mcl 主页：实例列表、模组管理、状态切换与一键启动",
        caption: "实例工作台 · 模组列表与一键启动",
      },
      {
        id: "market-mods",
        src: "/screenshots/market-mods.webp",
        alt: "mcl 资源市场：浏览 Modrinth 与 CurseForge 模组，支持多条件筛选",
        caption: "资源市场 · Modrinth / CurseForge 模组浏览",
      },
      {
        id: "market-detail",
        src: "/screenshots/market-detail.webp",
        alt: "mcl 模组详情页：项目介绍、版本历史与自动依赖规划",
        caption: "项目详情 · 更新日志与依赖自动规划",
      },
      {
        id: "creation",
        src: "/screenshots/creation.webp",
        alt: "mcl 新建实例：五种像素图标与加载器环境选择",
        caption: "新建实例 · 原版与四大模组加载器",
      },
      {
        id: "creation-import",
        src: "/screenshots/creation-import.webp",
        alt: "mcl 整合包导入：支持 Modrinth、CurseForge、MultiMC 与已有目录",
        caption: "整合包导入 · .mrpack / CurseForge / Prism",
      },
      {
        id: "market-modpacks",
        src: "/screenshots/market-modpacks.webp",
        alt: "mcl 整合包浏览：直接检索与安装流行整合包",
        caption: "整合包市场 · 检索与受限资源手动补齐",
      },
      {
        id: "market-shaders",
        src: "/screenshots/market-shaders.webp",
        alt: "mcl 光影包列表：按 Iris、OptiFine 等环境筛选",
        caption: "光影 / 资源包 / 数据包 · 统一分类浏览",
      },
      {
        id: "settings-game",
        src: "/screenshots/settings-game.webp",
        alt: "mcl 游戏设置：物理内存自动配额、分辨率、GC 策略与目录管理",
        caption: "游戏设置 · 自动内存上限与实时内存条",
      },
      {
        id: "settings-java",
        src: "/screenshots/settings-java.webp",
        alt: "mcl Java 管理：并发探测本机 JDK 并支持一键下载 Temurin",
        caption: "Java 管理 · 本机探测与 Temurin 下载",
      },
      {
        id: "accounts",
        src: "/screenshots/accounts.webp",
        alt: "mcl 账户页面：支持微软 OAuth 正版登录与离线 ID",
        caption: "账户体系 · 微软正版 OAuth 与离线 ID",
      },
    ],
    about: {
      id: "about",
      title: "为什么选择 mcl",
      body: "放弃网页套壳与繁杂冗余的配置面板。mcl 把注意力集中在现代架构、文件完整性以及克制干净的日常操作上。",
      items: [
        {
          icon: "bolt",
          title: "纯 Go + 原生 UI",
          body: "基于 MyGo 驱动原生桌面界面与字体渲染，无 Chromium、无 Node 运行时，启动即刻响应。",
        },
        {
          icon: "apple",
          title: "macOS 与 Windows 原生",
          body: "发布已签名公证的 Apple Silicon / Intel DMG 与 Windows x64 / ARM64 便携可执行文件。",
        },
        {
          icon: "package",
          title: "Modrinth & CurseForge",
          body: "统一浏览双平台模组、光影、资源包、数据包与整合包，支持加载器与运行环境多选过滤。",
        },
        {
          icon: "layers",
          title: "全加载器集成",
          body: "原生支持 Vanilla、Fabric、Forge、NeoForge 与 Quilt，含现代 spec 1 客户端处理器执行。",
        },
        {
          icon: "puzzle",
          title: "依赖分析与诊断",
          body: "离线解析本地 JAR 的 Fabric / Quilt / Forge / NeoForge 声明，停用或删减前先检查依赖关系。",
        },
        {
          icon: "search",
          title: "指纹识别本地资源",
          body: "通过流式 SHA-1 与 CurseForge 指纹批量反查旧有模组来源身份，零 guessing，严格按摘要落库。",
        },
        {
          icon: "coffee",
          title: "自动 Java 与 Temurin",
          body: "自动探测本机 JDK；缺失对应版本时从 Eclipse Temurin 官方源校验 SHA-256 并自动落盘。",
        },
        {
          icon: "sliders",
          title: "自动内存与五种 GC",
          body: "依据系统物理内存自动计算合理上限，内置 G1GC / ZGC / Shenandoah 等参数预设并兼容 Java 23+。",
        },
        {
          icon: "lock",
          title: "系统文件锁与恢复",
          body: "安装、更新与进程交接全程受文件级互斥锁与私有暂存目录保护，不直接覆盖写入，支持撤销。",
        },
        {
          icon: "shield",
          title: "纯净可审计的 MIT 开源",
          body: "无广告、无推广位、无内置闭源代理；所有核心日志自动脱敏，确保账号令牌不外泄。",
        },
      ],
    },
    showcases: [
      {
        id: "market",
        title: "双源资源市场，一眼找到想要的",
        body: "Modrinth 与 CurseForge 共享统一的过滤语法、详情排版与版本时间线。浏览与下载互不阻塞，支持随时暂停或取消任务。",
        items: [
          {
            title: "多维过滤与项目详情",
            body: "按目标实例版本、加载器、类别及客户端/服务端运行环境多选筛选；内置限长安全的 Markdown 简介阅读、完整更新日志与文件摘要展示。",
            image: "/screenshots/market-mods.webp",
            alt: "mcl 资源市场模组列表截图",
            backdrop: "/screenshots/market-shaders.webp",
            bullets: [
              { icon: "search", label: "Modrinth 与 CurseForge 双源切换" },
              { icon: "layers", label: "自动剔除不匹配的旧版与加载器" },
              { icon: "bolt", label: "流式计算必需依赖计划" },
              { icon: "shield", label: "写盘前全程核验 SHA-1 / SHA-512" },
            ],
          },
          {
            title: "严格分发边界，安全发布到实例",
            body: "遇到 CurseForge 第三方分发受限的文件，绝不绕过限制或伪造请求，而是提供清晰的手动补齐向导：按固定摘要校验你本地下载的文件后再安全入库。",
            image: "/screenshots/market-detail.webp",
            alt: "mcl Sodium 项目详情页面截图",
            backdrop: "/screenshots/home.webp",
            bullets: [
              { icon: "lock", label: "非覆盖暂存与重名防冲突" },
              { icon: "package", label: "支持模组、光影、资源包与数据包" },
              { icon: "sliders", label: "固定版本更新并保留停用状态" },
              { icon: "check", label: "操作出错自动从备份日志回滚" },
            ],
          },
        ],
      },
      {
        id: "modpacks",
        title: "实例管理与整合包工作流",
        body: "每个实例拥有完全隔离的游戏目录与配置文件。无论从零搭建原版，还是导入百余个模组的大型整合包，全部保持秩序井然。",
        items: [
          {
            title: "创建、导入与五大加载器图标",
            body: "支持在线搜索下载、本地导入 `.mrpack` / CurseForge 整合包或 MultiMC / Prism 实例。附带五枚原汁原味的手工像素画预设图标，也支持上传本地图片并后台归一化为 80×80 PNG。",
            image: "/screenshots/creation.webp",
            alt: "mcl 新建实例界面截图",
            backdrop: "/screenshots/creation-import.webp",
            bullets: [
              { loader: "grass", label: "Vanilla 原版" },
              { loader: "fabric", label: "Fabric" },
              { loader: "forge", label: "Forge" },
              { loader: "neoforge", label: "NeoForge" },
              { loader: "quilt", label: "Quilt" },
            ],
          },
          {
            title: "本地资产管理与依赖保护",
            body: "一键启用、停用或撤销移除实例内的任何模组、光影或数据包；内置 `core/mods` 元数据解析引擎，提前阻止意外拆散前置模组，还能用文件指纹批量识别未记录来源的历史文件。",
            image: "/screenshots/home.webp",
            alt: "mcl 实例管理与模组列表截图",
            backdrop: "/screenshots/market-modpacks.webp",
            bullets: [
              { icon: "puzzle", label: "原生读取 fabric.mod.json & mods.toml" },
              { icon: "search", label: "批量 SHA-1 / CurseForge 指纹识别" },
              { icon: "bolt", label: "会话内随时撤销错误删除" },
              { icon: "shield", label: "先检查环境冲突再落盘更新" },
            ],
          },
        ],
      },
      {
        id: "runtime",
        title: "无感运行环境与安全启动链",
        body: "不再手算 `-Xmx` 参数，也不用到处找对应的 JDK。从探测环境、校验资源到进程交接，一切都在并发锁与清晰状态提示中完成。",
        items: [
          {
            title: "自动适配 Java 与物理内存",
            body: "启动前按 Minecraft 目标大版本与本机 CPU 架构自动选择稳定的 JDK；缺省时直连 Eclipse Temurin 官方源下载校验安装。同时按物理内存自动规划启动配额，支持 G1GC / ZGC 等五种参数策略。",
            image: "/screenshots/settings-java.webp",
            alt: "mcl Java 管理页面截图",
            backdrop: "/screenshots/settings-game.webp",
            bullets: [
              { icon: "coffee", label: "并行探测本机最多 256 个 Java 候选" },
              { icon: "download", label: "Eclipse Temurin SHA-256 原子发布" },
              { icon: "cpu", label: "物理内存四分之一智能限额（2–6 GiB）" },
              { icon: "sliders", label: "Java 23+ ZGC 自动切换参数兼容" },
            ],
          },
          {
            title: "正版 OAuth 认证与可靠进程交接",
            body: "支持微软正版账户（浏览器 OAuth2 PKCE）与离线 ID，绝不加载外部 Java Agent 注入游戏。通过 `StartTracked` 写好追踪锁后交接游戏窗口状态，游戏日志对敏感 Token 自动脱敏。",
            image: "/screenshots/accounts.webp",
            alt: "mcl 账户登录界面截图",
            backdrop: "/screenshots/settings-download.webp",
            bullets: [
              { icon: "user", label: "微软官方 OAuth 与离线 ID 共存" },
              { icon: "lock", label: "全链路下载缓存与系统级实例文件锁" },
              { icon: "terminal", label: "macOS Quartz 实时探测首个可见游戏窗" },
              { icon: "shield", label: "凭据仅落入本地受保护存储" },
            ],
          },
        ],
      },
    ],
    opensource: {
      id: "opensource",
      title: "真正的开源软件",
      body: "mcl 使用宽松的 MIT 协议在 GitHub 公开全部源码。没有任何隐藏闭源模块，你可以随时查阅下载校验、线程锁与启动器的每一行代码。",
      starsLabel: "GitHub Stars",
      forksLabel: "Forks",
      licenseLabel: "开源协议",
      licenseValue: "MIT License",
      viewRepoLabel: "查看 GitHub 源码",
      issuesLabel: "提交反馈 / Issue",
      can: {
        label: "你可以",
        items: [
          "按 MIT 协议自由阅读、修改、重新分发源码或自行发布二进制版本",
          "克隆仓库后借助官方工具链脚本完全本地编译与运行测试",
          "在本地完全离线或自配 `MCL_MICROSOFT_CLIENT_ID` / `MCL_CURSEFORGE_API_KEY`",
          "检查每一次下载的 SHA-1 / SHA-256 校验，或者随时提出 PR 改进代码",
        ],
      },
      wont: {
        label: "我们不会",
        items: [
          "绝不植入启动页广告、推广链接、捐赠强制弹窗或遥测统计",
          "绝不借用、泄露第三方客户端 ID，也不绕过作者禁止分发的资源限制",
          "绝不注入任何 Java Agent、篡改游戏字节码或破坏正版认证链",
          "绝不把核心能力拆分进闭源动态库、后台常驻守护进程或云端鉴权墙",
        ],
      },
    },
    faq: {
      id: "faq",
      title: "常见问题",
      items: [
        {
          q: "为什么选择 Go 与 MyGo 开发原生客户端？",
          a: "网页技术栈启动快、开发方便，但启动器本质上是一个高频读写文件、管理并发下载与调起外部进程的工具。Go 拥有极佳的并发原语与单文件静态分发能力，配合 MyGo 原生 UI 库，可以在几百毫秒内冷启动且保持很小的体积与极低内存占用。",
        },
        {
          q: "微软正版登录为什么提示需要 Client ID？",
          a: "为了遵循微软官方 OAuth 应用安全规范、同时避免在开源仓库中借用或泄露其他项目的密钥，开发构建版本支持通过环境变量 `MCL_MICROSOFT_CLIENT_ID` 或应用数据目录下的 `.env.local` 配置你自己的 Azure 应用标识；正式发行的 GitHub Release 已随构建流水线配置好官方登录入口。",
        },
        {
          q: "CurseForge 下载为什么需要 API Key，受限模组怎么办？",
          a: "根据 CurseForge 官方开发者条款，软件不能内置公开密钥或绕过模组作者设定的“不允许第三方启动器自动下载”选项。你可以在市场设置页仅在内存中临时填入自有 API Key；遇到受限文件时，mcl 会给出对应网页并在你手动下载后按固定指纹和摘要完成核验并安装。",
        },
        {
          q: "mcl 会注入 Java Agent 或外置皮肤补丁吗？",
          a: "不会。mcl 在设计边界中明确排除任何注入式 Java Agent、启动时篡改游戏类字节码或拦截验证服务的行为，保持游戏环境原汁原味与可预测。",
        },
        {
          q: "目前支持哪些操作系统？未来会有 Linux 版吗？",
          a: "当前每个 GitHub Release 会自动构建并签名公证 macOS（Apple Silicon `arm64` / Intel `amd64` `.dmg`），以及构建 Windows（`x64` / `ARM64` `.exe`）。Linux 上的窗口后端正在随底层工具链演进观察中，你也可以参阅文档尝试从源码探索构建。",
        },
        {
          q: "如何验证我下载的安装包没有被篡改？",
          a: "每次正式发布都会同时附带 `SHA256SUMS` 校验文件，并且 macOS 版本已经过 Apple Developer 签名与公证（Notarization）。下载完成后你可以在终端用 `shasum -a 256 -c` 核对摘要。",
        },
      ],
    },
    footer: {
      watermark: "Meow Craft Launcher",
      disclaimer:
        "本项目与 Mojang Studios 及 Microsoft 无关。Minecraft 是 Mojang Synergies AB 的商标。",
      mitLine: "MIT 开源软件 · Go + MyGo 原生构建",
      docsLink: "使用文档",
      githubLink: "GitHub 仓库",
      releasesLink: "发布版本",
    },
    docs: {
      siteLabel: "mcl 使用文档",
      homeLink: "返回官网",
      browseMenu: "文档目录",
      searchPlaceholder: "搜索文档...",
      searchHint: "/",
      emptySearch: "没有找到匹配的文档章节。",
      onThisPage: "本页目录",
      previous: "上一篇",
      next: "下一篇",
      copy: "复制",
      copied: "已复制",
      copyCode: "复制",
      copiedCode: "已复制",
    },
  },
  en: {
    lang: "en",
    title: "mcl — Meow Craft Launcher | Lightweight Native Minecraft Launcher",
    description:
      "Lightweight native Minecraft: Java Edition launcher written in Go with MyGo native UI. Dual-source market, five loaders, modpack workflows, and automatic Java resolution under the MIT license.",
    skip: "Skip to content",
    brand: "mcl",
    brandFull: "Meow Craft Launcher",
    homePath: "/",
    docsPath: "/docs/",
    otherLocale: "zh",
    otherLocaleLabel: "中文",
    otherHomePath: "/zh/",
    railNavLabel: "On this page",
    menuOpenLabel: "Open navigation",
    menuCloseLabel: "Close navigation",
    nav: [
      { id: "about", label: "Overview", icon: "layers" },
      { id: "market", label: "Market", icon: "package" },
      { id: "modpacks", label: "Instances & Packs", icon: "puzzle" },
      { id: "runtime", label: "Runtime & Safety", icon: "cpu" },
      { id: "opensource", label: "Open Source", icon: "code" },
      { id: "faq", label: "FAQ", icon: "help" },
    ],
    hero: {
      prefix: "Built for craft,",
      suffix: "made for Minecraft.",
      body: "Lightweight native Minecraft: Java Edition launcher written in Go with MyGo native UI. No Electron, no Tauri, no bundled web runtime.",
      downloadMacArm: "Download for macOS (Apple Silicon)",
      downloadWinX64: "Download for Windows (x64)",
      downloadLabel: "Download",
      otherDownloadsLabel: "Choose platform release",
      allReleasesLabel: "All Releases & SHA256SUMS",
      buildFromSourceLabel: "Build from Source",
      docsLabel: "Read Docs",
      starLabel: "Star",
      prevShot: "Previous screenshot",
      nextShot: "Next screenshot",
      closeLightbox: "Close preview",
    },
    downloads: [
      {
        id: "darwin-arm64",
        platform: "macos",
        name: "macOS (Apple Silicon)",
        meta: `v${LATEST_TAG} · Signed & Notarized .dmg`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-darwin-arm64.dmg`,
      },
      {
        id: "darwin-amd64",
        platform: "macos",
        name: "macOS (Intel x86_64)",
        meta: `v${LATEST_TAG} · Signed & Notarized .dmg`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-darwin-amd64.dmg`,
      },
      {
        id: "windows-amd64",
        platform: "windows",
        name: "Windows (x64 Portable)",
        meta: `v${LATEST_TAG} · Single .exe binary`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-windows-amd64.exe`,
      },
      {
        id: "windows-arm64",
        platform: "windows",
        name: "Windows (ARM64 Portable)",
        meta: `v${LATEST_TAG} · Single .exe binary`,
        url: `${RELEASES_URL}/latest/download/mcl-${LATEST_TAG}-windows-arm64.exe`,
      },
    ],
    screenshots: [
      {
        id: "home",
        src: "/screenshots/home.webp",
        alt: "mcl home workspace showing instance sidebar, mods list, toggle controls and launch button",
        caption: "Instance Workspace · Assets & Quick Launch",
      },
      {
        id: "market-mods",
        src: "/screenshots/market-mods.webp",
        alt: "mcl Market browsing Modrinth mods with loader, category and environment filters",
        caption: "Resource Market · Modrinth & CurseForge Filtering",
      },
      {
        id: "market-detail",
        src: "/screenshots/market-detail.webp",
        alt: "mcl Sodium mod detail view showing Markdown overview, versions and homepage link",
        caption: "Project Detail · Changelogs & Dependency Planning",
      },
      {
        id: "creation",
        src: "/screenshots/creation.webp",
        alt: "mcl New Instance view with pixel art loader presets and version picker",
        caption: "New Instance · Vanilla & Four Mod Loaders",
      },
      {
        id: "creation-import",
        src: "/screenshots/creation-import.webp",
        alt: "mcl Import tab supporting Modrinth .mrpack, CurseForge zip and MultiMC/Prism",
        caption: "Pack Import · .mrpack / CurseForge / Prism",
      },
      {
        id: "market-modpacks",
        src: "/screenshots/market-modpacks.webp",
        alt: "mcl Modpack catalog powered by Modrinth and CurseForge",
        caption: "Modpacks · Integrated Catalog & Local Completion",
      },
      {
        id: "market-shaders",
        src: "/screenshots/market-shaders.webp",
        alt: "mcl Shaders browsing view with Iris and OptiFine filters",
        caption: "Shaders / Resource Packs / Datapacks",
      },
      {
        id: "settings-game",
        src: "/screenshots/settings-game.webp",
        alt: "mcl Game Settings with live memory gauge, GC options and resolution",
        caption: "Game Settings · Automatic Memory & Live Meter",
      },
      {
        id: "settings-java",
        src: "/screenshots/settings-java.webp",
        alt: "mcl Java Manager showing detected JDKs and Eclipse Temurin downloader",
        caption: "Java Runtime · Auto Discovery & Temurin Downloads",
      },
      {
        id: "accounts",
        src: "/screenshots/accounts.webp",
        alt: "mcl Accounts screen with Microsoft OAuth login and Offline ID controls",
        caption: "Accounts · Official Microsoft OAuth & Offline IDs",
      },
    ],
    about: {
      id: "about",
      title: "Why use mcl",
      body: "Switching a launcher takes patience. mcl is engineered around native responsiveness, verifiable file operations, and a clean surface without web bloat.",
      items: [
        {
          icon: "bolt",
          title: "Go + MyGo Native UI",
          body: "Compiled single binary driving native graphics and text rendering directly—zero Chromium, zero Node, fast startup.",
        },
        {
          icon: "apple",
          title: "macOS & Windows",
          body: "Official releases ship notarized Apple Silicon & Intel DMGs alongside portable Windows x64 and ARM64 binaries.",
        },
        {
          icon: "package",
          title: "Dual-Source Market",
          body: "Search and install mods, shaders, resource packs, datapacks, and modpacks across Modrinth and CurseForge.",
        },
        {
          icon: "layers",
          title: "Five Loaders Supported",
          body: "First-class Vanilla, Fabric, Forge, NeoForge, and Quilt setup including modern spec-1 Forge client processors.",
        },
        {
          icon: "puzzle",
          title: "Dependency Diagnostics",
          body: "Reads local JAR metadata cleanly offline to catch missing prerequisites, conflicts, or disabled dependencies.",
        },
        {
          icon: "search",
          title: "Fingerprint Matching",
          body: "Streams SHA-1 and CurseForge hashes to identify pre-existing mod files strictly by digest—never guessing by filename.",
        },
        {
          icon: "coffee",
          title: "Auto Java & Temurin",
          body: "Scans local JDKs concurrently and fetches verified Eclipse Temurin builds automatically if a required major version is missing.",
        },
        {
          icon: "sliders",
          title: "Smart Memory & 5 GCs",
          body: "Computes balanced -Xmx allocations from physical RAM and adjusts ZGC flags cleanly through Java 23+.",
        },
        {
          icon: "lock",
          title: "File Locks & Atomic Publish",
          body: "OS-level instance locks, isolated staging directories, and undoable deletes guard every write against races.",
        },
        {
          icon: "shield",
          title: "Clean MIT Open Source",
          body: "No ads, no telemetry, no hidden agents. Download streams check SHA-1/SHA-256 and logs redact sensitive tokens.",
        },
      ],
    },
    showcases: [
      {
        id: "market",
        title: "Browse Modrinth & CurseForge without friction",
        body: "Unified multi-select filtering for loaders, categories, and client/server environments—backed by cancellable tasks and verified disk staging.",
        items: [
          {
            title: "Filtered Search & Rich Project Views",
            body: "Filter directly against your target instance's Minecraft version and loader. Read bounded Markdown overviews, inspect version changelogs, and preview required dependency plans before writing anything to disk.",
            image: "/screenshots/market-mods.webp",
            alt: "mcl Market mods list in dark mode",
            backdrop: "/screenshots/market-shaders.webp",
            bullets: [
              { icon: "search", label: "Modrinth & CurseForge side by side" },
              { icon: "layers", label: "Multi-select loaders & environments" },
              { icon: "bolt", label: "Automatic required dependency resolution" },
              { icon: "shield", label: "Hash-checked staged downloads" },
            ],
          },
          {
            title: "Strict Distribution Rules & Safe Publishing",
            body: "When CurseForge projects restrict automated downloads, mcl never bypasses limits or scrapes private proxies—it verifies your locally downloaded file by pinned digest and size before safely publishing.",
            image: "/screenshots/market-detail.webp",
            alt: "mcl Sodium mod project overview",
            backdrop: "/screenshots/home.webp",
            bullets: [
              { icon: "lock", label: "Non-overwriting isolated file staging" },
              { icon: "package", label: "Mods, shaders, packs & CurseForge worlds" },
              { icon: "sliders", label: "Version updates preserve disabled state" },
              { icon: "check", label: "Automatic rollback from transaction journal" },
            ],
          },
        ],
      },
      {
        id: "modpacks",
        title: "Isolated Instances & Modpack Workflows",
        body: "Keep every version and modded setup cleanly separated, whether built from scratch or imported from a community pack.",
        items: [
          {
            title: "Create, Import & Hand-Crafted Pixel Presets",
            body: "Provision clean Vanilla instances, wire up Fabric / Forge / NeoForge / Quilt, or import `.mrpack`, CurseForge archives, and existing MultiMC / Prism setups—paired with custom 16×16 pixel sprites or your own 80×80 PNG icon.",
            image: "/screenshots/creation.webp",
            alt: "mcl New Instance creation screen",
            backdrop: "/screenshots/creation-import.webp",
            bullets: [
              { loader: "grass", label: "Vanilla" },
              { loader: "fabric", label: "Fabric" },
              { loader: "forge", label: "Forge" },
              { loader: "neoforge", label: "NeoForge" },
              { loader: "quilt", label: "Quilt" },
            ],
          },
          {
            title: "Asset Controls, Identity & Local Checks",
            body: "Toggle or remove installed files with session undo, trace unknown files by SHA-1 / CurseForge fingerprint, and verify `fabric.mod.json` / `mods.toml` relationships offline before changes apply.",
            image: "/screenshots/home.webp",
            alt: "mcl Instance detail showing managed assets",
            backdrop: "/screenshots/market-modpacks.webp",
            bullets: [
              { icon: "puzzle", label: "Offline Fabric, Quilt, Forge & NeoForge checks" },
              { icon: "search", label: "Strict SHA-1 / size fingerprint matching" },
              { icon: "bolt", label: "Undoable asset removal inside the session" },
              { icon: "shield", label: "Pre-flight safety checks before toggling" },
            ],
          },
        ],
      },
      {
        id: "runtime",
        title: "Zero-Guesswork Java & Tracked Launch",
        body: "Skip manual `-Xmx` math and hunt-and-peck JDK installs. mcl pairs automatic runtime selection with OS locks and clean authentication.",
        items: [
          {
            title: "Auto Java Resolution & Memory Sizing",
            body: "Before launching, mcl matches the exact required JDK major version and CPU architecture on your system—or downloads an official Eclipse Temurin package verified by SHA-256. Memory scales safely from physical RAM alongside 5 GC strategies.",
            image: "/screenshots/settings-java.webp",
            alt: "mcl Java Manager window",
            backdrop: "/screenshots/settings-game.webp",
            bullets: [
              { icon: "coffee", label: "Concurrent local JDK discovery" },
              { icon: "download", label: "Verified Eclipse Temurin installer" },
              { icon: "cpu", label: "Quarter-RAM allocation (2–6 GiB cap)" },
              { icon: "sliders", label: "G1GC, ZGC & Shenandoah compatibility" },
            ],
          },
          {
            title: "Microsoft OAuth & Process Handoff",
            body: "Sign in via official browser OAuth (`MCL_MICROSOFT_CLIENT_ID`) or manage Offline profiles with zero bytecode agents. Tracked process handoff watches the live game window and redacts secrets from session logs.",
            image: "/screenshots/accounts.webp",
            alt: "mcl Accounts management screen",
            backdrop: "/screenshots/settings-download.webp",
            bullets: [
              { icon: "user", label: "Official Microsoft OAuth & Offline IDs" },
              { icon: "lock", label: "OS file locks prevent concurrent corruption" },
              { icon: "terminal", label: "macOS Quartz detection for first game window" },
              { icon: "shield", label: "Token redaction across logs & diagnostics" },
            ],
          },
        ],
      },
    ],
    opensource: {
      id: "opensource",
      title: "Genuinely Open Source",
      body: "mcl is licensed cleanly under MIT on GitHub. Every line of download verification, mod metadata parsing, locking, and native UI lives openly in the repository.",
      starsLabel: "GitHub Stars",
      forksLabel: "Forks",
      licenseLabel: "License",
      licenseValue: "MIT License",
      viewRepoLabel: "View Source on GitHub",
      issuesLabel: "Report an Issue",
      can: {
        label: "What you can do",
        items: [
          "Inspect, fork, modify, and redistribute the codebase or builds under MIT terms",
          "Compile everything reproducibly from source using the bundled Go toolchain wrapper",
          "Supply your own Microsoft Client ID or session-only CurseForge API Key",
          "Audit every hash check, lockfile, and launcher argument—or contribute a PR",
        ],
      },
      wont: {
        label: "What we won't do",
        items: [
          "Ship banner ads, sponsor popups, promoted modpacks, or analytics beacons",
          "Borrow third-party client IDs, bundle leaked keys, or bypass author restrictions",
          "Inject external Java agents, rewrite game classes, or intercept auth checks",
          "Hide features inside closed binaries, background daemons, or paywalled tiers",
        ],
      },
    },
    faq: {
      id: "faq",
      title: "Frequently Asked Questions",
      items: [
        {
          q: "How is mcl built under the hood?",
          a: "mcl is written in Go with MyGo for native desktop UI rendering—meaning no embedded web browser, Chromium process tree, or JavaFX launcher shell. Core packages remain strictly separated from UI and platform OS adapters.",
        },
        {
          q: "Why does Microsoft sign-in mention a custom Client ID?",
          a: "mcl never borrows or bundles another project's Azure client ID. Official CI release builds provide Microsoft login directly, while local development builds let you supply your own personal app ID via `MCL_MICROSOFT_CLIENT_ID` or an ignored `.env.local` file.",
        },
        {
          q: "Do I need a CurseForge API key, and what about restricted downloads?",
          a: "Modrinth works out of the box with no key. For CurseForge, you can paste your own API key in-memory inside the Market panel or set `MCL_CURSEFORGE_API_KEY`. When a mod author disables automated downloads, mcl guides you to download it manually and validates the exact file hash before installing.",
        },
        {
          q: "Does mcl modify the Minecraft client or inject Java agents?",
          a: "No. mcl intentionally excludes external Java agents, bytecode patching, or authlib-style injectors so game launches remain clean and faithful to official manifests.",
        },
        {
          q: "Which platforms are supported?",
          a: "Pre-built releases target macOS (signed and notarized Apple Silicon `arm64` and Intel `amd64` DMGs) and Windows (`x64` and `ARM64` portable `.exe` binaries). See the Building from Source guide in Docs for toolchain details.",
        },
        {
          q: "How do I verify downloaded releases?",
          a: "Every GitHub Release includes a `SHA256SUMS` manifest, and macOS DMGs are code-signed and notarized with Apple Developer credentials.",
        },
      ],
    },
    footer: {
      watermark: "Meow Craft Launcher",
      disclaimer:
        "Not affiliated with Mojang Studios or Microsoft. Minecraft is a trademark of Mojang Synergies AB.",
      mitLine: "Open-source MIT Software · Native Go + MyGo",
      docsLink: "Documentation",
      githubLink: "GitHub",
      releasesLink: "Releases",
    },
    docs: {
      siteLabel: "mcl Docs",
      homeLink: "Website",
      browseMenu: "Docs Menu",
      searchPlaceholder: "Filter docs...",
      searchHint: "/",
      emptySearch: "No documentation pages match your filter.",
      onThisPage: "On this page",
      previous: "Previous",
      next: "Next",
      copy: "Copy",
      copied: "Copied",
      copyCode: "Copy",
      copiedCode: "Copied",
    },
  },
};
