# Custom Command Input Design

Date: 2026-02-18

## Problem

The `custom` tool type exists in the codebase and `db.ToolCommand` already supports
passing an arbitrary command string for it, but there is no UI to specify that command.
Selecting `custom` in the new session or edit session dialog today is functionally
identical to `shell` -- both launch `/bin/bash`.

## Goal

When the user selects `custom` in the Tool dropdown (new session or edit session
dialog), a "Command" input field appears. When any other tool is selected, the
field is hidden. The command is persisted to the `sessions.command` column and
used when the tmux session is created or restarted.

## Approach: Dynamic add/remove (Option A)

Use tview's `Form.AddFormItem` / `Form.RemoveFormItem` to insert or remove the
Command input field at index 2 (immediately after the Tool dropdown) when the
selection changes. A closure variable `commandShown bool` tracks current state.

## Files Changed

### `internal/ui/dialogs/new.go`

- Add `Command string` to `NewSessionResult`.
- Add `commandShown bool` closure variable and a `setCommandVisible(show bool)`
  helper that adds/removes the Command `InputField` at index 2.
- Call `setCommandVisible` from the Tool dropdown's `SetSelectedFunc`.
- On submit, read Command only when tool is `custom`.

### `internal/ui/dialogs/edit_session.go`

- Add `Command string` to `EditSessionResult`.
- Same dynamic show/hide logic as above.
- Pre-populate Command with `s.Command` when `s.Tool == db.ToolCustom` so
  existing custom sessions retain their command.
- On submit, read Command only when tool is `custom`.

### `internal/session/session.go`

- Add `Command string` to `UpdateOptions`.
- In `Update()`, replace the hardcoded `db.ToolCommand(opts.Tool, "")` with
  `db.ToolCommand(opts.Tool, opts.Command)` so the custom string is forwarded.

### `internal/ui/app.go`

- Pass `result.Command` into `session.UpdateOptions.Command` when handling
  `EditSessionResult`.
- New session creation already passes `Command` through `CreateOptions.Command`;
  add `result.Command` there too.

## No DB Changes

`command` is already stored in the `sessions` table. No migration needed.

## Error Handling

If the user selects `custom` but leaves Command blank, `db.ToolCommand` falls
back to `/bin/bash` -- existing behavior, acceptable.
