---
title: 文档概览
description: Meow Craft Launcher (mcl) 的整体介绍、原生架构特点与文档结构说明。
section: 快速开始
sectionOrder: 1
order: 0
---

**Meow Craft Launcher (`mcl`)** 是一款使用 **Go** 与 **[MyGo](https://github.com/egoist/mygo)** 开发的原生 Minecraft Java 版桌面启动器。

它致力于让游戏实例管理保持快速、可预期与纯粹，不依赖浏览器内核，也没有捆绑与广告。

## 核心能力

- **隔离实例** — 每个实例拥有独立的模组、资源包、光影、存档与加载器配置，互不干扰。
- **五大运行环境** — 支持原版（Vanilla）、Fabric、Quilt、Forge 与 NeoForge。
- **内置资源市场** — 直接浏览、筛选并安装来自 Modrinth 与 CurseForge 的模组、光影、资源包、数据包和整合包。
- **整合包导入与修复** — 支持导入 Modrinth（`.mrpack`）与 CurseForge（`.zip`）整合包，并为受限分发文件提供本地校验补齐。
- **自动 Java 与内存处理** — 自动探测本机 Java 运行环境，按版本与架构匹配，必要时自动安装官方 Eclipse Temurin，并自动分配内存上限。
- **统一账户体系** — 支持微软官方 OAuth 登录与离线玩家档案。

## 文档结构

1. **快速开始** — 安装包说明、SHA-256 校验、账户登录与创建第一个实例。
2. **核心功能** — 实例与目录管理、资源市场、整合包与加载器、Java 与启动设置。
3. **参考与开发** — 环境变量、源码构建与诊断排查。

> 文档框架已就绪。后续只需在 `website/src/content/docs/` 下新增或修改 Markdown 文件即可自动生成对应页面与目录导航。
