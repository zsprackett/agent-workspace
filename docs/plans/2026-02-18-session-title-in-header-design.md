# Design: Session Title in Tmux Status Bar Header

**Date:** 2026-02-18

## Summary

Add the current session's title to the tmux status bar header, displayed between the app name and the session stats, separated by a pipe character.

## Current Behavior

```
 AGENT WORKSPACE  ● 1 running  ◐ 1 waiting  2 total
```

## Desired Behavior

```
 AGENT WORKSPACE  my-session-title | ● 1 running  ◐ 1 waiting  2 total
```

## Design

**File:** `internal/tmux/tmux.go`

- `writeStatsFile(path string, running, waiting, total int)` gains a `title string` parameter.
- Format string updated to include the title between the app name and the pipe-separated counts:
  ```
  #[fg=#89b4fa,bold] AGENT WORKSPACE #[fg=#cdd6f4,nobold] <title> #[fg=#6c7086]| ● %d running  ◐ %d waiting  %d total
  ```
- All three `writeStatsFile` call sites inside `AttachSession` pass the existing `title` variable (already in scope).

No other files require changes.
