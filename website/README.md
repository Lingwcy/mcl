# mcl website

Static marketing and documentation site for **mcl (Meow Craft Launcher)**, built with Astro.

- **Landing page**: `/` (English) and `/zh/` (Simplified Chinese)
- **Documentation**: `/docs/` (English) and `/zh/docs/` (Simplified Chinese)

## Commands

Run inside `website/`:

```sh
pnpm install
pnpm dev
pnpm typecheck
pnpm build
pnpm preview
```

Deploy `website/dist/` to any static host. Optionally set `SITE_URL` (for example `https://mcl.example.com`) at build time to generate canonical URLs, `hreflang` alternates, and Open Graph URLs.

## Writing documentation

Documentation pages live under:

- `src/content/docs/en/` (English)
- `src/content/docs/zh/` (Simplified Chinese)

Each Markdown file uses frontmatter validated by `src/content.config.ts`:

```yaml
---
title: Page Title
description: One-sentence summary shown below the title and in metadata.
section: Core Features
sectionOrder: 2
order: 1
---
```

Matching relative paths between `en/` and `zh/` (for example `features/instances.md`) automatically link together via the top-bar language switcher.
