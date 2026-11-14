---
title: 环境变量与配置
description: 支持的环境变量、下载设置选项与本地凭据存放规则。
section: 参考与开发
sectionOrder: 3
order: 1
---

## 环境变量

| 变量名 | 用途 | 说明 |
| --- | --- | --- |
| `MCL_MICROSOFT_CLIENT_ID` | 微软 OAuth 应用 Client ID | 从源码构建且需启用微软登录时提供；也可写入应用数据目录的 `.env.local`。 |
| `MCL_CURSEFORGE_API_KEY` | CurseForge Core API Key | 可选；设置后无需在市场配置页手动输入即可请求 CurseForge 接口。 |

## 下载设置

在 **设置 → 下载** 中可以调整以下选项：

- **下载源 / 镜像偏好**
- **最大并发数**
- **全局限速**
- **代理配置**
