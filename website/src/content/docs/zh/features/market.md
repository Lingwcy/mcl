---
title: 资源市场与模组管理
description: 浏览 Modrinth 与 CurseForge 资源、识别本地文件、诊断依赖并安全更新资产。
section: 核心功能
sectionOrder: 2
order: 2
---

通过侧边栏的 **市场** 入口，可以直接搜索并安装社区资源到指定实例。

## 支持的资源源与分类

- **平台来源**：Modrinth 与 CurseForge。
- **资源分类**：模组、光影、资源包、数据包与整合包。
- **多维筛选**：支持加载器、类别、运行环境多选，排序方式以及目标游戏版本过滤。

> **CurseForge API Key 说明**：Modrinth 无需任何配置即可独立使用。如需使用 CurseForge 搜索与元数据，可通过环境变量 `MCL_CURSEFORGE_API_KEY` 提供，或在市场 CurseForge 配置页临时输入（仅保留在内存中）。

## 本地资产管理与依赖诊断

在实例的**资产列表**中，`mcl` 会记录已安装文件的来源信息与加密摘要：

- **开关与可撤销移除** — 一键启用/停用普通文件，或移除至实例 `removed-assets` 目录并支持撤销。
- **本地识别** — 通过流式 SHA-1 与 CurseForge 指纹精确匹配本地文件，恢复来源项目信息，不依赖文件名猜测。
- **依赖诊断** — 结合平台元数据与 JAR 内嵌声明（`fabric.mod.json`、`quilt.mod.json`、`mods.toml`、`neoforge.mods.toml`），离线检查缺失、停用、重复、版本不符与冲突依赖。
- **安全更新** — 资产更新在私有暂存区下载并校验后非覆盖发布，同时保留原有的启停状态。
