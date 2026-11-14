---
title: Building from Source
description: Requirements and commands for running mcl locally or packaging releases with Go and MyGo.
section: Reference
sectionOrder: 3
order: 2
---

## Requirements

- **Go 1.27.3** or later
- **MyGo CLI** (pinned as a Go tool dependency in `go.mod`)

## Running in development

From the repository root:

```sh
go tool mygo dev
```

## Release metadata

Before tagging a release:

1. Update `"version"` in `mygo.json` (format `X.Y.Z`).
2. Add a matching `## X.Y.Z` section at the top of `CHANGELOG.md` (the in-app updater displays this section, and CI rejects builds without it).
3. Commit and push the matching `vX.Y.Z` tag:

```sh
git tag v0.0.7
git push origin main v0.0.7
```

GitHub Actions builds macOS (`darwin/arm64`, `darwin/amd64`) and Windows (`windows/amd64`, `windows/arm64`) packages with `CGO_ENABLED=0`, verifies binary architectures, and publishes `SHA256SUMS`.
