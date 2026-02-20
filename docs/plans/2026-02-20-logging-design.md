# Logging Infrastructure Design

Date: 2026-02-20

## Problem

TLS handshake errors from Go's `net/http` package (and other background goroutine errors) are
printed to stderr, which bleeds through the tview TUI into the terminal. The errors are valid
but should not appear on the console.

## Goal

Structured, file-backed logging with daily rotation and configurable log level. No new external
dependencies.

## Approach

stdlib `slog` for structured logging + a custom `dailyRotator` writer for daily file rotation.

## Components

### `internal/applog/applog.go` (new)

**`dailyRotator`** - mutex-protected `io.Writer`:
- Stores current date string (YYYY-MM-DD) and open `*os.File`
- On each `Write`: if date has changed or file is nil, closes current file, opens
  `<logDir>/agent-workspace-YYYY-MM-DD.log`, prunes files beyond 7 (alphabetical sort,
  delete oldest)
- `Close()` flushes and closes the current file

**`Init(cfg InitConfig) (*slog.Logger, error)`** - called once at startup:
- Creates the log dir if needed
- Constructs a `dailyRotator`
- Parses log level string ("debug"/"info"/"warn"/"error") to `slog.Level`
- Builds `slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: level})`
- Calls `slog.SetDefault(logger)` for package-wide default
- Calls `log.SetOutput(rotator)` and `log.SetFlags(0)` so stdlib `log` (and thus
  `net/http` TLS errors) also route to the file

### `internal/config/config.go`

Add to `Config`:
```go
LogLevel string `json:"logLevel"` // default "info"
LogDir   string `json:"logDir"`   // default ~/.agent-workspace/logs
```

Add `LogPath()` function returning `~/.agent-workspace/logs`.

### `main.go`

Call `applog.Init` after config loads, before opening the DB. Fatal errors before init
(tmux check, dir creation) stay on stderr - logging is not yet set up at that point.

### Packages receiving `*slog.Logger`

| Package | Logger use |
|---|---|
| `monitor` | debug-level status change events |
| `syncer` | warn-level on fetch errors (replaces `log.Printf`) |
| `notify` | warn-level on webhook POST errors (currently silent) |

Pattern: logger passed as final constructor parameter. No global logger variable.

### Packages unchanged

`session`, `git`, `db`, `tmux` return errors to callers - no fire-and-forget logging.
`ui` surfaces errors via modal dialogs.
`notes`/`menu` subcommands are short-lived interactive popups - stderr is appropriate.

## File Changes

| File | Change |
|---|---|
| `internal/applog/applog.go` | New: `dailyRotator`, `Init` |
| `internal/applog/applog_test.go` | New: rotation, pruning, level parsing tests |
| `internal/config/config.go` | Add `LogLevel`, `LogDir` fields; `LogPath()` |
| `main.go` | Call `applog.Init`; pass logger to constructors |
| `internal/monitor/monitor.go` | Accept `*slog.Logger` |
| `internal/syncer/syncer.go` | Accept `*slog.Logger`; replace `log.Printf` |
| `internal/notify/notify.go` | Accept `*slog.Logger`; log webhook errors |

## Non-goals

- Log rotation by size (daily is sufficient)
- Structured logging in UI dialogs or subcommands
- Remote log shipping
