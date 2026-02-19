# Group Default Tool Design

**Date:** 2026-02-18

**Goal:** Move tool selection defaults up to the group level. Sessions inherit the group's default tool but can override it individually.

## Approach

Group gets a `DefaultTool` field. When creating a new session, the Tool dropdown pre-selects the group's tool. If the group has no tool set, fall back to `cfg.DefaultTool`, then to `"claude"`. Sessions continue to store their own tool, so per-session override works as today.

## Data Layer

- Add `default_tool TEXT NOT NULL DEFAULT ''` to the `groups` table via `ALTER TABLE` migration (empty string = no preference).
- Add `DefaultTool db.Tool` to `db.Group`.
- Update `SaveGroups`, `LoadGroups`, and `scanGroup` to include the new field.

## Group UI

- `GroupDialog` gains a Tool dropdown: `["(none)", "claude", "opencode", "gemini", "codex", "shell", "custom"]`. `"(none)"` maps to the empty string in storage.
- `GroupResult` gains `DefaultTool string`.
- `onNewGroup` and `onEdit` (group path) pass `DefaultTool` through to `SaveGroups`.

## New Session Dialog

- `NewSessionDialog` receives the selected group's `DefaultTool` alongside `cfg.DefaultTool`.
- Resolution order for the Tool dropdown's initial selection: group `DefaultTool` > `cfg.DefaultTool` > `"claude"`.
- When the user changes the Group dropdown, the Tool dropdown updates to reflect the new group's default.

## Edit Session Dialog

No change. Tool dropdown already present for per-session override.
