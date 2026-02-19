# Remote Access Design

**Date:** 2026-02-18
**Status:** Approved

## Goal

Close the remote access, notifications, and agent observability gaps with Paseo while staying single-binary, no external infrastructure required. Phone-first, same-network by default, Tailscale for anywhere access.

## Architecture

Today the binary runs the TUI plus two background goroutines (monitor, syncer). A third goroutine is added:

```
main.go → config + db + ui.NewApp
                        ↓
         ┌──────────────┼──────────────┐
      monitor        syncer         webserver
      (500ms)        (2min)         (HTTP + WS)
         │                              │
         └──── status updates ──────────┘
                    shared db
```

The `webserver` goroutine starts an HTTP listener when the TUI launches. If the port is already in use, log a warning and continue -- the TUI still works, web access is just unavailable.

The TUI and web clients are peers: both read from SQLite and receive monitor events. No shared in-process state beyond the DB.

## Components

### 1. `internal/webserver/`

HTTP + WebSocket server.

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Serves PWA (index.html from embed.FS) |
| `GET` | `/static/*` | JS/CSS/icons from embed.FS |
| `GET` | `/api/sessions` | All sessions + groups as JSON snapshot |
| `POST` | `/api/sessions/{id}/notes` | Update notes for a session |
| `GET` | `/api/sessions/{id}/events` | Activity log for a session |
| `WS` | `/ws` | WebSocket, server-push only |

**WebSocket events (server → client):**

```json
{ "type": "snapshot", "sessions": [ ... ] }
{ "type": "status_changed", "session_id": 42, "status": "waiting", "title": "swift-fox" }
{ "type": "session_created", "session": { ... } }
{ "type": "session_deleted", "session_id": 42 }
{ "type": "notes_updated", "session_id": 42, "notes": "..." }
```

A `snapshot` event is sent immediately on WebSocket connect. Subsequent events are incremental patches.

The monitor calls `webserver.Broadcast(event)` via an interface to avoid import cycles.

### 2. `internal/webserver/static/`

PWA embedded via `embed.FS`. Vanilla JS/HTML/CSS -- no build step, no npm.

```
static/
  index.html      app shell
  app.js          WebSocket client, state, rendering
  style.css       mobile-first layout, status colors
  manifest.json   PWA manifest (display: standalone)
  icon-192.png
  icon-512.png
```

**UI layout:**

```
┌─────────────────────────────┐
│  agent-workspace        ⚙   │
├─────────────────────────────┤
│  group-name                 │
│  ● swift-fox   claude  [n]  │
│  ◐ brave-owl   opencode     │
│                             │
│  another-group              │
│  ○ calm-deer   gemini       │
└─────────────────────────────┘
```

- Status dots match TUI icons
- Tapping a session expands to show notes and activity log inline
- WebSocket reconnects with exponential backoff
- "Add to Home Screen" on iOS: app icon, full-screen, no browser chrome
- v1 scope: read + notes only, no create/delete

### 3. ntfy Integration (`internal/notify/`)

Adds ntfy as a third notification channel alongside macOS alerts and webhook.

**Two modes:**

| Mode | URL example | Notes |
|------|-------------|-------|
| Self-hosted | `http://your-mac.local:8088/agent-workspace` | Same network only, zero external deps |
| ntfy.sh public | `https://ntfy.sh/<random-uuid>` | Works from anywhere, topic is public |

**POST format:**

```json
{
  "topic": "agent-workspace",
  "title": "swift-fox is waiting",
  "message": "claude · my-sessions",
  "priority": 4,
  "tags": ["rotating_light"]
}
```

Failure is non-fatal: log and continue.

### 4. Activity Log

**New DB table:**

```sql
CREATE TABLE session_events (
  id         INTEGER PRIMARY KEY,
  session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  ts         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  event_type TEXT NOT NULL,
  detail     TEXT
);
```

`event_type` values: `status_changed`, `notes_updated`, `created`, `stopped`, `restarted`, `detached`.

`detail` is a JSON blob with event-specific fields (e.g. `{"from": "running", "to": "waiting"}`).

**Populated by:**
- `monitor` -- status transitions
- `session.Manager` -- create, stop, restart, delete, detach

**TUI surface:** scrollable log in the right-column session detail panel.

**Web UI surface:** shown in expanded session row on tap.

## Configuration

```json
{
  "webserver": {
    "enabled": true,
    "port": 8080,
    "host": "0.0.0.0"
  },
  "notifications": {
    "enabled": true,
    "webhook": "https://hooks.slack.com/...",
    "ntfy": "http://localhost:8088/agent-workspace"
  }
}
```

`host: "0.0.0.0"` binds to all interfaces. Combined with Tailscale installed on the Mac, the web UI is reachable at `http://your-mac.tailnet-name.ts.net:8080` from any network.

## Tailscale

No code changes required. Document in README:

1. Install Tailscale on the Mac running agent-workspace
2. Install Tailscale on phone
3. Access via `http://your-mac.tailnet-name.ts.net:8080`

For ntfy with Tailscale: run ntfy on the same Mac, access via Tailscale hostname.

## Out of Scope (v1)

- Create/delete/stop sessions from web UI
- AI-generated session summaries
- Native mobile app
- Voice/dictation
- Authentication (rely on network-level access control)
