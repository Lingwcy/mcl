---
title: Accounts & First Launch
description: Sign in with a Microsoft account or add an offline profile, then create and launch your first instance.
section: Getting Started
sectionOrder: 1
order: 2
---

Open the **Accounts** panel from the avatar button near the bottom-left corner of the sidebar.

## Microsoft account

1. Select **Microsoft** in the Accounts view.
2. Complete the OAuth sign-in flow in your system browser (`http://localhost/oauth/callback`).
3. Wait for `mcl` to finish exchanging credentials and select the active profile.

> If you compile `mcl` yourself, supply your own Microsoft Entra application client ID via `MCL_MICROSOFT_CLIENT_ID` or `.env.local`. See [Environment & Configuration](/docs/reference/configuration/).

## Offline profile

1. Select **Offline** in the Accounts view.
2. Enter a player name to create a local profile for offline single-player or LAN testing.

## Creating your first instance

1. Click **+** in the instances sidebar to open the creation workspace.
2. Choose a loader (**Vanilla**, **Fabric**, **Quilt**, **Forge**, or **NeoForge**) and Minecraft version.
3. Click **Create & Install**, then press **Launch** once preparation finishes.
