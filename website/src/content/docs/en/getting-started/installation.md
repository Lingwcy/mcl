---
title: Installation
description: Download signed macOS disk images or portable Windows executables and verify SHA-256 checksums.
section: Getting Started
sectionOrder: 1
order: 1
---

Prebuilt desktop releases are published on [GitHub Releases](https://github.com/Lingwcy/mcl/releases/latest).

## Release artifacts

| Platform | Architecture | Artifact | Notes |
| --- | --- | --- | --- |
| macOS | Apple Silicon (`arm64`) | `mcl-<version>-arm64.dmg` | Signed and notarized |
| macOS | Intel (`amd64`) | `mcl-<version>-amd64.dmg` | Signed and notarized |
| Windows | x64 (`amd64`) | `mcl-<version>-windows-amd64.exe` | Portable executable |
| Windows | ARM64 (`arm64`) | `mcl-<version>-windows-arm64.exe` | Portable executable |

> On Windows, `amd64` refers to **x86-64 / x64** (64-bit Intel and AMD processors), not ARM64 or 32-bit x86.

## Verifying checksums

Every release publishes a `SHA256SUMS` file alongside the installers.

### macOS

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing
```

### Windows (PowerShell)

```powershell
Get-FileHash .\mcl-<version>-windows-amd64.exe -Algorithm SHA256
```

Compare the printed hash with the matching entry in `SHA256SUMS`.

## Automatic updates

Release builds include signed update manifests (`update-<platform>-<arch>.json`) and delta archives so installed launchers can check for and apply updates from GitHub Releases.
