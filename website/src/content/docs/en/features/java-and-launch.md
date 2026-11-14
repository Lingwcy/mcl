---
title: Java & Launch Settings
description: Automatic Java discovery, Eclipse Temurin downloads, memory allocation, GC policies, and launch monitoring.
section: Core Features
sectionOrder: 2
order: 4
---

`mcl` handles Java selection and launch arguments automatically before handing execution off to the game process.

## Java management

Open **Settings → Java** to inspect discovered runtimes or manage installations:

- **System discovery** — Scans common installation paths and probes each `java` binary to verify major version, vendor, architecture, and compiler availability.
- **Eclipse Temurin downloads** — Install official Temurin JDK or JRE builds directly into `mcl`'s managed `java` directory with SHA-256 verification.
- **Automatic resolution on launch** — When launching an instance, `mcl` picks a compatible runtime matching the required Java major version and architecture, automatically downloading Temurin if no compatible local runtime is found.

## Automatic memory allocation

Memory limits are calculated automatically during launch preparation:

- **64-bit Java**: One quarter of system physical memory, clamped between `2048 MiB` and `6144 MiB`.
- **32-bit Java**: `1024 MiB`.
- **Fallback**: `2048 MiB` if physical memory cannot be determined.

## Game settings & GC strategies

Under **Settings → Game**, you can configure the garbage collection strategy (five built-in presets, with automatic ZGC generational flag compatibility for Java 23+) and validated custom JVM arguments. Changes save automatically in the background.

## Launch workspace

Pressing **Launch** enters a dedicated preparation view that tracks asset verification, Java selection, session handoff, live process/window state, and sanitized session logs.
