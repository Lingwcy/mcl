---
title: Resource Market
description: Browse Modrinth and CurseForge, manage dependencies, recognize local files, and update installed assets safely.
section: Core Features
sectionOrder: 2
order: 2
---

The **Market** tab lets you browse and install community content directly into an instance.

## Supported sources and categories

- **Sources**: Modrinth and CurseForge.
- **Categories**: Mods, Shaders, Resource Packs, Data Packs, and Modpacks.
- **Filtering**: Multi-select loaders, categories, environments, sort order, and target Minecraft versions.

> **CurseForge API key**: Modrinth works out of the box without any key. For CurseForge, provide your own API key via `MCL_CURSEFORGE_API_KEY` or enter it in the in-app CurseForge configuration view (kept in memory only).

## Asset management & diagnostics

Inside an instance's **Assets** tab, `mcl` tracks installed files and source metadata:

- **Toggle & remove** — Enable, disable, or undoably remove managed files (`removed-assets`).
- **Local identification** — Match existing files by cryptographic hash (SHA-1 / CurseForge fingerprint) to recover project metadata without guessing by filename.
- **Dependency diagnostics** — Detect missing, disabled, duplicate, incompatible, or conflicting dependencies using both platform metadata and embedded mod declarations (`fabric.mod.json`, `quilt.mod.json`, `mods.toml`, `neoforge.mods.toml`).
- **Safe updates** — Stage compatible version updates in a private directory and publish non-destructively while preserving disabled states.
