# Meow Craft Launcher

English · [简体中文](README.zh-CN.md)

![Meow Craft Launcher showing the instance library, resource market, instance setup, and launch screen](media/readme-hero-v3.jpg)

**mcl** is a native Minecraft Java Edition launcher built with Go and MyGo. Create isolated instances, browse mods and resource packs, and manage Java runtimes from one app.

## Development

Requires Go 1.27.3 or later.

```sh
go tool mygo dev
```

## Releases

Set `version` in `mygo.json`, commit the change, then push the matching tag.

```sh
git tag v0.0.1
git push origin main v0.0.1
```

CI publishes a macOS universal DMG (Apple Silicon and Intel), a Windows x64 installer, and SHA-256 checksums. The `amd64` suffix means **x86-64 / x64** for 64-bit Intel and AMD Windows, not ARM64 or 32-bit x86. CI checks the actual PE architecture and hash of both the built app and the installer payload, rather than trusting the filename. The macOS app uses ad-hoc signing. Windows installers are unsigned. Manually running **Desktop release** builds downloadable artifacts.
