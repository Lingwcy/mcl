---
title: Diagnostics & Troubleshooting
description: How mcl captures session logs, prevents instance lock conflicts, and recovers interrupted tasks.
section: Reference
sectionOrder: 3
order: 3
---

## Session logs

During every launch, `mcl` records bounded session logs and automatically redacts sensitive tokens before saving a copy for inspection in the **Logs** view.

## Instance locks

`mcl` acquires OS-level file locks and tracks active game processes across launcher restarts so two instances or repair operations do not corrupt the same game directory simultaneously.

If a game process is still running when you reopen `mcl`, the instance stays marked as active and repair/deletion operations are blocked until the session exits.

## Interrupted updates & migrations

- **Asset updates** use private staging directories and a pending journal (`asset-update.pending.json`); reopening the instance view lets you finish or roll back recovery safely.
- **Directory migrations** preserve the previous copy until completion is confirmed.
- **Download tasks** persist state across restarts so incomplete transfers can be inspected or retried from the Downloads panel.
