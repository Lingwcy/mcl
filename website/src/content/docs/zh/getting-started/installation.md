---
title: 安装与平台支持
description: 下载经签名公证的 macOS 镜像或 Windows 便携版，并校验 SHA-256 摘要。
section: 快速开始
sectionOrder: 1
order: 1
---

预构建的桌面端发布包统一托管在 [GitHub Releases](https://github.com/Lingwcy/mcl/releases/latest)。

## 发布产物

| 平台 | 架构 | 文件名 | 说明 |
| --- | --- | --- | --- |
| macOS | Apple Silicon (`arm64`) | `mcl-<version>-arm64.dmg` | 已签名并公证 |
| macOS | Intel (`amd64`) | `mcl-<version>-amd64.dmg` | 已签名并公证 |
| Windows | x64 (`amd64`) | `mcl-<version>-windows-amd64.exe` | 便携可执行文件 |
| Windows | ARM64 (`arm64`) | `mcl-<version>-windows-arm64.exe` | 便携可执行文件 |

> Windows 文件名中的 `amd64` 表示 **x86-64 / x64**（适用于 64 位 Intel 与 AMD 处理器），并非 ARM64，也不适用于 32 位 Windows（x86）。

## 校验 SHA-256 摘要

每次发布都会在附件中附带 `SHA256SUMS` 校验清单。

### macOS

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing
```

### Windows (PowerShell)

```powershell
Get-FileHash .\mcl-<version>-windows-amd64.exe -Algorithm SHA256
```

将输出的摘要值与 `SHA256SUMS` 中对应条目核对一致即可。

## 自动更新

正式发布包附带签名的更新清单（`update-<platform>-<arch>.json`）与增量更新包，已安装的启动器可直接检查并完成版本更新。
