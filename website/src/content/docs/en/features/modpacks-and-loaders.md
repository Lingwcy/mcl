---
title: Modpacks & Loaders
description: Supported mod loaders, Modrinth and CurseForge archive imports, and missing-file completion.
section: Core Features
sectionOrder: 2
order: 3
---

`mcl` separates ordinary instance loader selection from modpack-pinned environments while sharing the same underlying installation and verification pipeline.

## Supported loaders

- **Vanilla**
- **Fabric**
- **Quilt**
- **Forge** (legacy `1.7.10` / `1.12.2` and modern client processors)
- **NeoForge**

## Importing modpacks

You can import `.mrpack` (Modrinth) and `.zip` (CurseForge manifest) archives either from the Market or from a local file on disk:

1. `mcl` inspects the archive, verifies hashes/boundaries, and shows optional files for selection.
2. Files are downloaded or matched into the digest cache before client overrides are applied.
3. If a CurseForge mod restricts third-party distribution, `mcl` lets you select the manually downloaded archive from your computer, verifies its exact size and hash, and resumes the same staging pipeline.
4. Imported modpacks also expose an instance-level **Repair / Missing Files** flow to restore missing pinned files or client overrides.
