---
title: 账户登录与首次启动
description: 使用微软账号或离线档案登录，并创建、启动你的第一个游戏实例。
section: 快速开始
sectionOrder: 1
order: 2
---

点击侧边栏左下角设置图标上方的头像入口，即可打开**账户**管理页。

## 微软官方账户

1. 在账户页选择 **微软登录**。
2. 在系统浏览器中完成 OAuth 登录授权（回调地址为 `http://localhost/oauth/callback`）。
3. 等待 `mcl` 完成凭据交换并自动选中当前游戏档案。

> 如果你是从源码自行编译 `mcl`，需要通过 `MCL_MICROSOFT_CLIENT_ID` 或应用数据目录下的 `.env.local` 提供自己的微软应用 Client ID，详见 [环境变量与配置](/zh/docs/reference/configuration/)。

## 离线档案

1. 在账户页选择 **离线档案**。
2. 输入玩家名称即可创建本地离线档案，用于单机或局域网联机测试。

## 创建并启动第一个实例

1. 点击实例侧边栏顶部的 **+** 进入新建工作台。
2. 选择运行环境（**原版**、**Fabric**、**Quilt**、**Forge** 或 **NeoForge**）以及 Minecraft 版本。
3. 点击 **创建并安装**，准备完成后点击 **启动** 即可进入游戏。
