---
title: Instances & Game Directories
description: Organize isolated Minecraft instances, discover existing installations, and relocate game directories safely.
section: Core Features
sectionOrder: 2
order: 1
---

Each instance in `mcl` keeps its own isolated configuration, assets, saves, and lock state so mods or loader versions never collide across profiles.

## Instance settings

Inside any instance view you can configure:

- **Name and icon** — Pick one of the built-in pixel presets or import a local image (resized and normalized into the instance directory).
- **Game location** — Keep the instance in the default launcher directory or bind it to a custom external game directory.
- **Java & launch preferences** — Override the preferred Java runtime or JVM arguments per instance.

## Discovering existing games

Use **Discover Existing Games** to scan common `.minecraft` directories or Swift Craft Launcher (`sjmcl`) profile layouts. Confirmed entries are copied into managed `mcl` instances while leaving the original folder untouched.

## Moving game directories

Instances can be migrated between built-in storage and external directories. `mcl` tracks migration state to allow safe recovery if interrupted, and keeps the previous copy until you choose to clean it up.
