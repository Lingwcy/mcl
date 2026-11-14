---
title: Environment & Configuration
description: Supported environment variables, download settings, and local credential rules.
section: Reference
sectionOrder: 3
order: 1
---

## Environment variables

| Variable | Purpose | Notes |
| --- | --- | --- |
| `MCL_MICROSOFT_CLIENT_ID` | Microsoft OAuth application Client ID | Required when building from source with Microsoft sign-in; can also live in `.env.local` inside the app data directory. |
| `MCL_CURSEFORGE_API_KEY` | CurseForge Core API key | Optional; enables CurseForge search and metadata requests without entering a key in the UI. |

## Download settings

In **Settings → Downloads**, you can tune:

- **Download source / mirror** preference
- **Concurrency** limits
- **Speed limit** throttling
- **HTTP / SOCKS proxy** configuration
