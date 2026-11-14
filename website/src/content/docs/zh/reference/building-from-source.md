---
title: 从源码构建
description: 本地运行与打包 Meow Craft Launcher 的工具链要求及发版流程。
section: 参考与开发
sectionOrder: 3
order: 2
---

## 环境要求

- **Go 1.27.3** 或更高版本
- **MyGo CLI**（已在 `go.mod` 中锁定为 Go tool 依赖）

## 本地开发运行

在仓库根目录执行：

```sh
go tool mygo dev
```

## 发版元数据约定

推送发版标签前需要完成以下步骤：

1. 在 `mygo.json` 中更新 `"version"`（格式 `X.Y.Z`）。
2. 在 `CHANGELOG.md` 顶部添加对应的 `## X.Y.Z` 小节（启动器内置更新窗口会读取此内容，缺少该小节时构建会失败）。
3. 提交修改并推送匹配的 `vX.Y.Z` 标签：

```sh
git tag v0.0.7
git push origin main v0.0.7
```

GitHub Actions 会以 `CGO_ENABLED=0` 构建 macOS（`darwin/arm64`、`darwin/amd64`）与 Windows（`windows/amd64`、`windows/arm64`）产物，校验真实二进制架构并生成 `SHA256SUMS`。
