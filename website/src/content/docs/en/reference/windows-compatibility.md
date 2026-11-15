---
title: Windows Compatibility
description: Verified Minecraft versions, mod loaders, automatic Java runtimes, mirror fallback behavior, and known boundaries on Windows 11 (AMD64).
section: Reference
sectionOrder: 3
order: 4
---

> This page **currently documents Windows (`Windows 11 AMD64`) only**. Real-machine compatibility testing for macOS and Linux has not started yet and is omitted until verified.

## Loader × Minecraft Version Matrix (Windows)

The matrix below summarizes real-machine compatibility across Minecraft version ranges and the five built-in runtime environments on Windows 11:

<div class="compat-legend" aria-label="Legend">
  <span class="compat-legend-item"><span class="compat-mark compat-ok" aria-hidden="true">✓</span> Supported (verified or shares the same pipeline)</span>
  <span class="compat-legend-item"><span class="compat-mark compat-no" aria-hidden="true">✕</span> Not supported / N/A</span>
  <span class="compat-legend-item"><span class="compat-mark compat-warn" aria-hidden="true">!</span> Unknown / blocked before end-to-end verification</span>
</div>

<div tabindex="0" role="region" aria-label="Loader × Minecraft Version Matrix (Windows)" class="compat-table-wrap">
  <table class="compat-table">
    <thead>
      <tr>
        <th scope="col">MC Version</th>
        <th scope="col">Vanilla</th>
        <th scope="col">Fabric</th>
        <th scope="col">Quilt</th>
        <th scope="col">Forge</th>
        <th scope="col">NeoForge</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <th scope="row">26.3 / 26.4-snap</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (26.3 &amp; 26.4-snapshot-8 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (26.3 / 0.30.2 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (26.3 / 66.1.8 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-warn" title="Unknown (catalog works; official .sha1 unreachable without proxy)"><span aria-hidden="true">!</span><span class="sr-only">Unknown</span></span></td>
      </tr>
      <tr>
        <th scope="row">26.1</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (0.30.0-beta.7 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (62.0.9 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-warn" title="Unknown (catalog and modpack files work; installer .sha1 blocked without proxy)"><span aria-hidden="true">!</span><span class="sr-only">Unknown</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.21.5 / 1.21.11</th>
        <td><span class="compat-mark compat-ok" title="Supported (1.21.11 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.21.5 / 55.1.0 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-warn" title="Unknown (not verified end-to-end)"><span aria-hidden="true">!</span><span class="sr-only">Unknown</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.21.1</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (0.16.7 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (0.29.2 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (52.1.10 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-warn" title="Unknown (not verified end-to-end)"><span aria-hidden="true">!</span><span class="sr-only">Unknown</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.20.1 / 1.20.4</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.20.1 / 0.15.10 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.20.1 / 0.26.0 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.20.1 / 47.4.29 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-warn" title="Unknown (catalog works; install not verified end-to-end)"><span aria-hidden="true">!</span><span class="sr-only">Unknown</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.17.1 – 1.19.4</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.18.2 / 40.2.21 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.14 – 1.16.5</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.16.5 / 0.18.5 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (shared Java 8 -cp fix)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.16.5 / 36.2.42 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.12.2</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (14.23.5.2859 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.7.10 / 1.8.9</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (1.7.10 &amp; 1.8.9 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.6.4</th>
        <td><span class="compat-mark compat-ok" title="Supported"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
        <td><span class="compat-mark compat-ok" title="Supported (standalone 9.11.1.1345 verified)"><span aria-hidden="true">✓</span><span class="sr-only">Supported</span></span></td>
        <td><span class="compat-mark compat-no" title="Not supported / N/A"><span aria-hidden="true">✕</span><span class="sr-only">Not supported</span></span></td>
      </tr>
    </tbody>
  </table>
</div>

## Verified Version Highlights

- **Vanilla**: End-to-end verified on `1.21.11`, with isolated legacy asset paths ready for historical releases (`1.6.4`–`1.12.2`).
- **Fabric**: Verified on `1.16.5` (`0.18.5`, Java 8 direct `-cp`), `1.20.1` (`0.15.10`), `1.21.1` (`0.16.7`), `26.3` (`0.19.5`), and `26.4-snapshot-8` (`0.20.1`).
- **Quilt**: Verified on `1.20.1` (`0.26.0`), `1.21.1` (`0.29.2`, bundled `mixinextras` handled), `26.1` (`0.30.0-beta.7`), and `26.3` (`0.30.2`); `1.14 – 1.16.5` shares the Java 8 `-cp` fix.
- **Forge**: Verified across `1.6.4` (standalone `9.11.1.1345`), `1.7.10`, `1.8.9` (`11.15.1.1902`), `1.12.2` (`14.23.5.2859`), `1.16.5` (`36.2.42`), `1.18.2` (`40.2.21`), `1.20.1` (`47.4.29`), `1.21.1` (`52.1.10`), `1.21.5` (`55.1.0`), `26.1` (`62.0.9`), and `26.3` (`66.1.8`).

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

- **Unsupported community loaders**: Only Vanilla, Fabric, Quilt, Forge, and NeoForge are supported; legacy or unbundled loaders such as `Legacy-Fabric`, `Babric`, `Ornithe`, and `LiteLoader` are outside scope.
- **Non-allowlisted download hosts**: External archive links outside the allowlisted official repositories are rejected.
- **Agents & third-party authentication**: `-javaagent:` flags, skin/cape patching agents, `authlib-injector`, and third-party auth providers are explicitly rejected.
