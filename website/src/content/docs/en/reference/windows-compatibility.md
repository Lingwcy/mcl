---
title: Windows Compatibility
description: Verified Minecraft versions, mod loaders, automatic Java runtimes, mirror fallback behavior, and known boundaries on Windows 11 (AMD64).
section: Reference
sectionOrder: 3
order: 4
---

> This page **currently documents Windows (`Windows 11 AMD64`) only**. Real-machine compatibility testing for macOS and Linux has not started yet and is omitted until verified.

## Loader × Minecraft Version Matrix (Windows)

The matrix below reflects live Windows 11 desktop-session runs verified through installation, digest/size validation, process launch handoff, and visible game window detection:

| MC Version | Vanilla | Fabric | Quilt | Forge | NeoForge | Legacy / Community Loaders |
| --- | --- | --- | --- | --- | --- | --- |
| `26.3` / `26.4-snap` | **Supported** (standard pipeline) | **Verified** (`26.3`, `26.4-snapshot-8`) | **Verified** (`26.3` / `0.30.2`) | **Verified** (`26.3` / `66.1.8`) | **Catalog verified / Install needs official `.sha1`** (see notes) | Not supported |
| `26.1` | **Supported** | **Supported** (same `26.3` pipeline) | **Verified** (`0.30.0-beta.7`) | **Verified** (`62.0.9`) | **Catalog & modpack verified / Install needs official `.sha1`** | Not supported |
| `1.21.5` / `1.21.11` | **Verified** (`1.21.11`) | **Supported** (Java 21 pipeline) | **Supported** (Java 21 pipeline) | **Verified** (`1.21.5` / `55.1.0`) | **Blocked without official `.sha1` access** | Not supported |
| `1.21.1` | **Supported** | **Verified** (`0.16.7`) | **Verified** (`0.29.2`) | **Verified** (`52.1.10`) | **Blocked without official `.sha1` access** | Not supported |
| `1.20.1` / `1.20.4` | **Supported** | **Verified** (`1.20.1` / `0.15.10`) | **Verified** (`1.20.1` / `0.26.0`) | **Verified** (`1.20.1` / `47.4.29`) | **Catalog verified / Install needs official `.sha1`** | Not supported |
| `1.17.1 – 1.19.4` | **Supported** | **Supported** (Java 17 pipeline) | **Supported** (Java 17 pipeline) | **Verified** (`1.18.2` / `40.2.21`) | N/A | Not supported |
| `1.13 – 1.16.5` | **Supported** | **Verified** (`1.16.5` / `0.18.5`) | **Supported** (shared Java 8 `-cp` fix) | **Verified** (`1.16.5` / `36.2.42`) | N/A | Not supported |
| `1.12.2` | **Supported** | Not supported | Not supported | **Verified** (`14.23.5.2859`) | N/A | Not supported |
| `1.7.10` / `1.8.9` | **Supported** (isolated legacy assets) | Not supported | Not supported | **Verified** (`1.7.10`, `1.8.9`) | N/A | Not supported |
| `1.6.4` | **Supported** (`legacy-assets` ready) | N/A | N/A | **Standalone verified** (`9.11.1.1345`; see legacy mod note) | N/A | Not supported |

## Automatic Java & Mirror Fallback Behavior

Under **Settings → Download**, the default download source is **Auto (`SourceAuto`)**, combining official endpoints with safe mirror fallback while keeping strict integrity checks intact:

- **Automatic Java installation (Windows)**: Tested end-to-end on Windows 11 for **Eclipse Temurin `8` / `17` / `21` / `25`** (including TUNA mirror fallback with `SHA-256` verification) and **Azul Zulu `8` / `17` / `21`** (automatic fallback with 100-byte catalog size-rounding tolerance and full `SHA-256` verification).
- **Fixed-artifact & catalog mirrors**: Supports BMCLAPI (for `Forge`, `NeoForge`, `Fabric`, and `Minecraft` version catalogs and `/maven` artifacts), Fabric-meta mirror fallback, and Aliyun Maven Central for immutable binary artifacts, with fast failover when an alternate candidate is available and no user rate limit is set.
- **Strict verification policy**: `mcl` **never trusts `.sha1` sidecar files from unofficial mirrors** and never self-hashes unverified bytes. For legacy Forge (`1.8.9`), when official `maven.minecraftforge.net` `.sha1` lookups time out, `mcl` only falls back to SHA-1-verified installer `checksums` or to official `repo.maven.apache.org/maven2/` `.sha1` metadata for non-Forge Maven Central dependencies.

## Known Boundaries & Notes

### NeoForge in networks without direct access to `maven.neoforged.net`

- With mirror fallback enabled, **NeoForge** (`1.20.1`, `1.20.4`, `26.1`, `26.3`) version listing and modpack file downloads succeed.
- However, resolving NeoForge installer checksums and unpinned library metadata still requires reaching official `.sha1` sidecars on `maven.neoforged.net`. On restricted networks without a proxy (where public mirrors do not serve trusted `.sha1` sidecars), `mcl` refuses to bypass digest verification and reports a loader connectivity error. Install NeoForge over a direct connection or working proxy.

### Legacy version & mod quirks observed on Windows

- **Third-party `Forge 1.6.4` modpacks**: Standalone `Forge 1.6.4` (`9.11.1.1345`) installs and opens a visible game window cleanly, whereas certain 2013 modpacks contain legacy mods (such as old `CodeChickenCorePlugin`) that crash during startup trying to fetch defunct remote libraries.
- **`Forge 1.12.2` with `fullscreen:true`**: Some `1.12.2` packs shipped with `fullscreen:true` in `options.txt` may exit on an unattended first startup during menu initialization and launch cleanly on retry.

## Explicitly Unsupported Scope

- **Legacy / niche community loaders**: `Legacy-Fabric`, `Babric`, `Ornithe`, and `LiteLoader` are not supported (only Vanilla, Fabric, Quilt, Forge, and NeoForge are supported).
- **Non-allowlisted download hosts**: External archive links outside the allowlisted official repositories are rejected.
- **Agents & third-party authentication**: `-javaagent:` flags, skin/cape patching agents, `authlib-injector`, and third-party auth providers are explicitly rejected.
