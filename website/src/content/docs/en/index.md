---
title: Introduction
description: Overview of Meow Craft Launcher (mcl), its native architecture, and how this documentation is organized.
section: Getting Started
sectionOrder: 1
order: 0
---

**Meow Craft Launcher (`mcl`)** is a native Minecraft: Java Edition desktop launcher built with **Go** and **[MyGo](https://github.com/egoist/mygo)**.

It is designed to keep Minecraft instance management fast, predictable, and self-contained without a browser runtime or bundled telemetry.

## What mcl includes

- **Isolated instances** — Each instance keeps its own mods, resource packs, shader packs, saves, and loader configuration.
- **Five runtime environments** — Vanilla, Fabric, Quilt, Forge, and NeoForge.
- **Integrated resource market** — Search, filter, and install mods, shaders, resource packs, data packs, and modpacks from Modrinth and CurseForge.
- **Modpack import & repair** — Import `.mrpack` (Modrinth) and `.zip` (CurseForge) archives into isolated instances, with local fallback for restricted CurseForge files.
- **Automatic Java & memory handling** — Probes local Java runtimes, installs matching Eclipse Temurin JDK/JRE builds when needed, and sets architecture-safe memory bounds automatically.
- **Unified accounts** — Supports official Microsoft OAuth sign-in alongside offline profiles.

## Documentation structure

1. **Getting Started** — Download installers, verify checksums, and set up your account.
2. **Core Features** — Work with instances, loaders, the resource market, and Java runtimes.
3. **Reference** — Environment variables, building from source, and diagnostics.

> This documentation framework is ready to expand. Edit or add Markdown files under `website/src/content/docs/` to publish new guides.
