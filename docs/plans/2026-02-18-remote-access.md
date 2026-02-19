# Remote Access Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an embedded HTTP server, PWA, ntfy notifications, and a session activity log to make agent-workspace accessible from a phone on the same network.

**Architecture:** A new `internal/webserver` package starts alongside the TUI as a goroutine. It serves a vanilla-JS PWA via `embed.FS` and pushes real-time updates via Server-Sent Events (SSE -- plain HTTP, no extra libraries). A `Broadcaster` interface in `internal/events` lets monitor and webserver communicate without import cycles. ntfy is added as a third notification channel. A `session_events` table records timestamped activity per session.

**Tech Stack:** Go stdlib (`net/http`, `embed`), vanilla JS/HTML/CSS (no build step), SQLite (existing), ntfy HTTP API.

---

### Task 1: Add WebserverConfig and NtfyURL to config

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go` (create if it doesn't exist):

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/config"
)

func TestLoadWebserverDefaults(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.Webserver.Enabled {
		t.Error("webserver should be enabled by default")
	}
	if cfg.Webserver.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Webserver.Port)
	}
	if cfg.Webserver.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Webserver.Host)
	}
}

func TestLoadNtfyURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"notifications":{"ntfy":"http://localhost:8088/aw"}}`), 0600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.NtfyURL != "http://localhost:8088/aw" {
		t.Errorf("expected ntfy URL, got %q", cfg.Notifications.NtfyURL)
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/config/... -run TestLoadWebserverDefaults -v
go test ./internal/config/... -run TestLoadNtfyURL -v
```

Expected: FAIL (fields don't exist yet)

**Step 3: Add the structs and fields**

In `internal/config/config.go`, add `WebserverConfig` struct and update `NotificationsConfig` and `Config`:

```go
type WebserverConfig struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Host    string `json:"host"`
}

type NotificationsConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	NtfyURL string `json:"ntfy"`
}

type Config struct {
	DefaultTool   string              `json:"defaultTool"`
	DefaultGroup  string              `json:"defaultGroup"`
	Worktree      WorktreeConfig      `json:"worktree"`
	ReposDir      string              `json:"reposDir"`
	WorktreesDir  string              `json:"worktreesDir"`
	Notifications NotificationsConfig `json:"notifications"`
	Webserver     WebserverConfig     `json:"webserver"`
}
```

Update `Defaults()` to set webserver defaults:

```go
func Defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		DefaultTool:  "claude",
		DefaultGroup: "my-sessions",
		Worktree:     WorktreeConfig{DefaultBaseBranch: "main"},
		ReposDir:     filepath.Join(home, ".agent-workspace", "repos"),
		WorktreesDir: filepath.Join(home, ".agent-workspace", "worktrees"),
		Webserver: WebserverConfig{
			Enabled: true,
			Port:    8080,
			Host:    "0.0.0.0",
		},
	}
}
```

**Step 4: Run tests to verify they pass**

```
go test ./internal/config/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add WebserverConfig and NtfyURL to config"
```

---

### Task 2: Add session_events table and DB methods

**Files:**
- Modify: `internal/db/types.go`
- Modify: `internal/db/db.go`

**Step 1: Write the failing test**

Add to `internal/db/db_test.go` (create if it doesn't exist):

```go
package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSessionEvents(t *testing.T) {
	store := openTestDB(t)

	// Insert a session so foreign key constraint is satisfied
	s := &db.Session{
		ID:          "test-1",
		Title:       "test",
		ProjectPath: "/tmp",
		GroupPath:   "default",
		Tool:        db.ToolClaude,
		Status:      db.StatusIdle,
		TmuxSession: "agws_test",
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatal(err)
	}

	if err := store.InsertSessionEvent("test-1", "created", ""); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}
	if err := store.InsertSessionEvent("test-1", "status_changed", `{"from":"idle","to":"running"}`); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}

	events, err := store.GetSessionEvents("test-1", 50)
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "status_changed" {
		t.Errorf("expected most recent first: got %q", events[0].EventType)
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/db/... -run TestSessionEvents -v
```

Expected: FAIL

**Step 3: Add SessionEvent type to types.go**

```go
type SessionEvent struct {
	ID        int64
	SessionID string
	Ts        time.Time
	EventType string
	Detail    string
}
```

**Step 4: Add migration and methods to db.go**

In `Migrate()`, after the existing group migrations, add:

```go
_, err = d.sql.Exec(`
    CREATE TABLE IF NOT EXISTS session_events (
        id         INTEGER PRIMARY KEY,
        session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
        ts         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
        event_type TEXT NOT NULL,
        detail     TEXT NOT NULL DEFAULT ''
    )
`)
if err != nil {
    return fmt.Errorf("create session_events: %w", err)
}

if _, alterErr := d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_session_events_session_id ON session_events(session_id, ts DESC)`); alterErr != nil {
    return fmt.Errorf("index session_events: %w", alterErr)
}
```

Add methods after the existing methods:

```go
func (d *DB) InsertSessionEvent(sessionID, eventType, detail string) error {
	_, err := d.sql.Exec(
		`INSERT INTO session_events (session_id, event_type, detail) VALUES (?, ?, ?)`,
		sessionID, eventType, detail,
	)
	return err
}

func (d *DB) GetSessionEvents(sessionID string, limit int) ([]SessionEvent, error) {
	rows, err := d.sql.Query(
		`SELECT id, session_id, ts, event_type, detail
		 FROM session_events
		 WHERE session_id = ?
		 ORDER BY ts DESC
		 LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SessionEvent
	for rows.Next() {
		var e SessionEvent
		var ts string
		if err := rows.Scan(&e.ID, &e.SessionID, &ts, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		e.Ts, _ = time.Parse("2006-01-02 15:04:05", ts)
		events = append(events, e)
	}
	return events, rows.Err()
}
```

**Step 5: Run tests to verify they pass**

```
go test ./internal/db/... -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/db/types.go internal/db/db.go internal/db/db_test.go
git commit -m "feat: add session_events table and DB methods"
```

---

### Task 3: Add ntfy notification channel

**Files:**
- Modify: `internal/notify/notify.go`

**Step 1: Write the failing test**

Create `internal/notify/notify_test.go`:

```go
package notify_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

func TestNtfyNotification(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n := notify.New(notify.Config{
		Enabled: true,
		NtfyURL: srv.URL + "/test-topic",
	})

	n.Notify(db.Session{
		ID:        "1",
		Title:     "swift-fox",
		Tool:      db.ToolClaude,
		GroupPath: "my-sessions",
		Status:    db.StatusWaiting,
	})

	if received == nil {
		t.Fatal("no POST received")
	}
	if received["title"] != "swift-fox is waiting" {
		t.Errorf("unexpected title: %v", received["title"])
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/notify/... -run TestNtfyNotification -v
```

Expected: FAIL

**Step 3: Add NtfyURL field and sendNtfy method**

Update `notify.go`:

```go
// Config holds notification settings.
type Config struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	NtfyURL string `json:"ntfy"`
}
```

Add to `Notify`:

```go
func (n *Notifier) Notify(s db.Session) {
	if !n.cfg.Enabled {
		return
	}
	msg := fmt.Sprintf("%s (%s) is waiting for input", s.Title, string(s.Tool))
	n.sendSystemNotification(msg)
	if n.cfg.Webhook != "" {
		n.sendWebhook(s)
	}
	if n.cfg.NtfyURL != "" {
		n.sendNtfy(s)
	}
}
```

Add the new method:

```go
type ntfyPayload struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
	Tags     []string `json:"tags"`
}

func (n *Notifier) sendNtfy(s db.Session) {
	payload := ntfyPayload{
		Title:    fmt.Sprintf("%s is waiting", s.Title),
		Message:  fmt.Sprintf("%s · %s", string(s.Tool), s.GroupPath),
		Priority: 4,
		Tags:     []string{"rotating_light"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(n.cfg.NtfyURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return
	}
	resp.Body.Close()
}
```

**Step 4: Run tests to verify they pass**

```
go test ./internal/notify/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/notify/notify.go internal/notify/notify_test.go
git commit -m "feat: add ntfy push notification channel"
```

---

### Task 4: Define Broadcaster interface

**Files:**
- Create: `internal/events/events.go`

**Step 1: Create the package** (no test needed -- it's a pure interface definition)

```go
package events

import "github.com/zsprackett/agent-workspace/internal/db"

// Event is a real-time update pushed to web clients.
type Event struct {
	Type      string        `json:"type"`
	SessionID string        `json:"session_id,omitempty"`
	Status    db.SessionStatus `json:"status,omitempty"`
	Title     string        `json:"title,omitempty"`
}

// Broadcaster sends events to connected web clients.
// A nil Broadcaster is safe to use -- Broadcast becomes a no-op.
type Broadcaster interface {
	Broadcast(e Event)
}
```

**Step 2: Verify it compiles**

```
go build ./internal/events/...
```

Expected: no errors

**Step 3: Commit**

```bash
git add internal/events/events.go
git commit -m "feat: add events.Broadcaster interface"
```

---

### Task 5: Record events and broadcast in monitor

**Files:**
- Modify: `internal/monitor/monitor.go`

**Step 1: Write the failing test**

Add to `internal/monitor/monitor_test.go` (create if needed):

```go
package monitor_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
	"github.com/zsprackett/agent-workspace/internal/monitor"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

type captureBroadcaster struct {
	events []events.Event
}

func (c *captureBroadcaster) Broadcast(e events.Event) {
	c.events = append(c.events, e)
}

func TestMonitorAcceptsBroadcaster(t *testing.T) {
	// Just verify the constructor accepts a Broadcaster without panicking.
	broadcaster := &captureBroadcaster{}
	notifier := notify.New(notify.Config{})

	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	store.Migrate()
	defer store.Close()

	mon := monitor.New(store, func() {}, notifier, broadcaster)
	if mon == nil {
		t.Fatal("expected non-nil monitor")
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/monitor/... -run TestMonitorAcceptsBroadcaster -v
```

Expected: FAIL (constructor signature doesn't match)

**Step 3: Update monitor.go**

Add import for `events` package and update struct and constructor:

```go
import (
	"encoding/json"
	"sync"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
	"github.com/zsprackett/agent-workspace/internal/notify"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

type Monitor struct {
	db          *db.DB
	onUpdate    OnUpdate
	notifier    *notify.Notifier
	broadcaster events.Broadcaster
	prevStatus  map[string]db.SessionStatus
	interval    time.Duration
	stop        chan struct{}
	wg          sync.WaitGroup
}

func New(store *db.DB, onUpdate OnUpdate, notifier *notify.Notifier, broadcaster events.Broadcaster) *Monitor {
	return &Monitor{
		db:          store,
		onUpdate:    onUpdate,
		notifier:    notifier,
		broadcaster: broadcaster,
		prevStatus:  make(map[string]db.SessionStatus),
		interval:    500 * time.Millisecond,
		stop:        make(chan struct{}),
	}
}
```

Add a helper at the bottom of the file:

```go
func (m *Monitor) broadcast(e events.Event) {
	if m.broadcaster != nil {
		m.broadcaster.Broadcast(e)
	}
}
```

In `refresh()`, inside the `if newStatus != s.Status` block, after `m.db.WriteStatus(...)`, add:

```go
detail, _ := json.Marshal(map[string]string{"from": string(s.Status), "to": string(newStatus)})
m.db.InsertSessionEvent(s.ID, "status_changed", string(detail))
m.broadcast(events.Event{
    Type:      "status_changed",
    SessionID: s.ID,
    Status:    newStatus,
    Title:     s.Title,
})
```

**Step 4: Fix the call site in ui/app.go**

In `internal/ui/app.go`, update the `monitor.New` call to pass `nil` for broadcaster (webserver doesn't exist yet):

```go
a.mon = monitor.New(store, func() {
    a.tapp.QueueUpdateDraw(func() {
        a.refreshHome()
    })
}, notifier, nil)
```

**Step 5: Run all tests**

```
go test ./...
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/monitor/monitor.go internal/monitor/monitor_test.go internal/ui/app.go
git commit -m "feat: monitor records status_changed events and broadcasts"
```

---

### Task 6: Record lifecycle events in session.Manager

**Files:**
- Modify: `internal/session/session.go`

**Step 1: Write the failing test**

Add to `internal/session/session_test.go` (check if it exists first -- add a new test):

```go
func TestManagerRecordsCreateEvent(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	store.Migrate()
	defer store.Close()

	mgr := session.NewManager(store)
	s, err := mgr.Create(session.CreateOptions{
		Title:     "swift-fox",
		GroupPath: "default",
		Tool:      db.ToolClaude,
		Cwd:       t.TempDir(),
	})
	if err != nil {
		// Create will fail without tmux -- just check the event logic
		t.Skip("tmux not available")
	}

	evts, err := store.GetSessionEvents(s.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evts {
		if e.EventType == "created" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'created' event")
	}
}
```

**Step 2: Run test to verify it is skipped or fails**

```
go test ./internal/session/... -run TestManagerRecordsCreateEvent -v
```

Expected: SKIP (no tmux in test env) or FAIL if tmux is available

**Step 3: Add event recording to session.go**

In the `Create` method, after the session is saved successfully (before return), add:

```go
_ = m.store.InsertSessionEvent(s.ID, "created", "")
```

In the `Stop` method, after stopping succeeds:

```go
_ = m.store.InsertSessionEvent(id, "stopped", "")
```

In the `Restart` method, after restart succeeds:

```go
_ = m.store.InsertSessionEvent(id, "restarted", "")
```

In the `Delete` method, before deleting (so FK still exists):

```go
_ = m.store.InsertSessionEvent(id, "deleted", "")
```

**Step 4: Run all tests**

```
go test ./...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: session.Manager records lifecycle events"
```

---

### Task 7: Webserver -- SSE hub and HTTP handlers

**Files:**
- Create: `internal/webserver/webserver.go`

**Step 1: Write the failing test**

Create `internal/webserver/webserver_test.go`:

```go
package webserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func TestSessionsEndpoint(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	store.Migrate()
	defer store.Close()

	srv := webserver.New(store, webserver.Config{Port: 0, Host: "127.0.0.1", Enabled: true})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result["sessions"]; !ok {
		t.Error("expected 'sessions' key in response")
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/webserver/... -run TestSessionsEndpoint -v
```

Expected: FAIL (package doesn't exist)

**Step 3: Create webserver.go**

```go
package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
)

type Config struct {
	Enabled bool
	Port    int
	Host    string
}

type Server struct {
	store   *db.DB
	cfg     Config
	mu      sync.Mutex
	clients map[chan events.Event]struct{}
}

func New(store *db.DB, cfg Config) *Server {
	return &Server{
		store:   store,
		cfg:     cfg,
		clients: make(map[chan events.Event]struct{}),
	}
}

// Broadcast implements events.Broadcaster.
func (s *Server) Broadcast(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Server) addClient(ch chan events.Event) {
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeClient(ch chan events.Event) {
	s.mu.Lock()
	delete(s.clients, ch)
	s.mu.Unlock()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions/{id}/notes", s.handleUpdateNotes)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSessionEvents)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.Handle("GET /", http.FileServer(staticFiles()))
	return mux
}

func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("webserver: %v\n", err)
		}
	}()
	return nil
}

type sessionsResponse struct {
	Sessions []*db.Session `json:"sessions"`
	Groups   []*db.Group   `json:"groups"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.LoadSessions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	groups, err := s.store.LoadGroups()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionsResponse{Sessions: sessions, Groups: groups})
}

func (s *Server) handleUpdateNotes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.store.UpdateSessionNotes(id, body.Notes); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "notes_updated", SessionID: id})
	w.WriteHeader(204)
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	evts, err := s.store.GetSessionEvents(id, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"events": evts})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	ch := make(chan events.Event, 16)
	s.addClient(ch)
	defer s.removeClient(ch)

	// Send initial snapshot event
	writeSSE(w, flusher, events.Event{Type: "snapshot"})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(w, flusher, e)
		case <-ticker.C:
			// keepalive comment
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, f http.Flusher, e events.Event) {
	data, _ := json.Marshal(e)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}
```

**Step 4: Run tests to verify they pass**

```
go test ./internal/webserver/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/webserver/webserver.go internal/webserver/webserver_test.go
git commit -m "feat: add webserver with SSE hub and HTTP handlers"
```

---

### Task 8: Embed PWA static files

**Files:**
- Create: `internal/webserver/static.go`
- Create: `internal/webserver/static/index.html`
- Create: `internal/webserver/static/app.js`
- Create: `internal/webserver/static/style.css`
- Create: `internal/webserver/static/manifest.json`

**Step 1: Create static.go (embed wrapper)**

```go
package webserver

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

func staticFiles() http.FileSystem {
	sub, _ := fs.Sub(staticFS, "static")
	return http.FS(sub)
}
```

**Step 2: Create static/manifest.json**

```json
{
  "name": "agent-workspace",
  "short_name": "agents",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#0d1117",
  "theme_color": "#0d1117",
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

**Step 3: Create static/style.css**

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg: #0d1117;
  --surface: #161b22;
  --border: #30363d;
  --text: #e6edf3;
  --muted: #8b949e;
  --running: #3fb950;
  --waiting: #d29922;
  --idle: #8b949e;
  --stopped: #6e7681;
  --error: #f85149;
}

body {
  font-family: ui-monospace, 'SF Mono', Menlo, monospace;
  font-size: 14px;
  background: var(--bg);
  color: var(--text);
  min-height: 100dvh;
}

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  background: var(--bg);
  z-index: 10;
}

header h1 { font-size: 14px; font-weight: 600; }

#connection-status {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--stopped);
}
#connection-status.connected { background: var(--running); }

#session-list { padding: 8px 0; }

.group-header {
  padding: 8px 16px 4px;
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.session-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  cursor: pointer;
  border-bottom: 1px solid transparent;
}
.session-row:hover { background: var(--surface); }
.session-row.expanded { background: var(--surface); border-bottom-color: var(--border); }

.status-dot { font-size: 16px; flex-shrink: 0; }
.status-dot.running  { color: var(--running); }
.status-dot.waiting  { color: var(--waiting); }
.status-dot.idle     { color: var(--idle); }
.status-dot.stopped  { color: var(--stopped); }
.status-dot.error    { color: var(--error); }

.session-title { flex: 1; font-weight: 500; }
.session-tool  { color: var(--muted); font-size: 12px; }
.dirty-flag    { color: var(--waiting); font-size: 11px; }

.session-detail {
  display: none;
  padding: 8px 16px 12px 42px;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}
.session-detail.open { display: block; }

.notes-area {
  width: 100%;
  min-height: 60px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 6px 8px;
  font-family: inherit;
  font-size: 13px;
  resize: vertical;
  margin-bottom: 6px;
}

.save-btn {
  font-size: 12px;
  padding: 4px 10px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
.save-btn:hover { border-color: var(--text); }

.event-log { margin-top: 10px; }
.event-log-title { font-size: 11px; color: var(--muted); margin-bottom: 4px; }
.event-entry {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--muted);
  padding: 2px 0;
}
.event-ts { flex-shrink: 0; }
.event-type { color: var(--text); }
```

**Step 4: Create static/app.js**

```js
const STATUS_ICONS = {
  running: { char: '●', cls: 'running' },
  waiting: { char: '◐', cls: 'waiting' },
  idle:    { char: '○', cls: 'idle' },
  stopped: { char: '◻', cls: 'stopped' },
  error:   { char: '✗', cls: 'error' },
};

let state = { sessions: [], groups: [] };
let expandedSessions = new Set();
let sseRetryDelay = 1000;

async function fetchSessions() {
  const res = await fetch('/api/sessions');
  if (!res.ok) return;
  state = await res.json();
  if (!state.sessions) state.sessions = [];
  if (!state.groups)   state.groups = [];
  render();
}

function connectSSE() {
  const es = new EventSource('/events');
  const dot = document.getElementById('connection-status');

  es.onopen = () => {
    dot.className = 'connected';
    sseRetryDelay = 1000;
  };
  es.onmessage = (e) => {
    const evt = JSON.parse(e.data);
    if (evt.type === 'snapshot' || evt.type === 'refresh') {
      fetchSessions();
    } else if (evt.type === 'status_changed') {
      const s = state.sessions.find(s => s.ID === evt.session_id);
      if (s) { s.Status = evt.status; render(); }
      else    { fetchSessions(); }
    } else {
      fetchSessions();
    }
  };
  es.onerror = () => {
    dot.className = '';
    es.close();
    setTimeout(connectSSE, sseRetryDelay);
    sseRetryDelay = Math.min(sseRetryDelay * 2, 30000);
  };
}

function formatTime(tsStr) {
  if (!tsStr) return '';
  const d = new Date(tsStr);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

async function loadEvents(sessionID, container) {
  const res = await fetch(`/api/sessions/${sessionID}/events`);
  if (!res.ok) return;
  const { events } = await res.json();
  if (!events || events.length === 0) return;

  const log = document.createElement('div');
  log.className = 'event-log';
  log.innerHTML = '<div class="event-log-title">Activity</div>';
  events.slice(0, 10).forEach(e => {
    const row = document.createElement('div');
    row.className = 'event-entry';
    row.innerHTML = `<span class="event-ts">${formatTime(e.Ts)}</span><span class="event-type">${e.EventType}</span>`;
    log.appendChild(row);
  });
  container.appendChild(log);
}

async function saveNotes(sessionID, notes) {
  await fetch(`/api/sessions/${sessionID}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  });
}

function render() {
  const list = document.getElementById('session-list');
  list.innerHTML = '';

  // Group sessions by group path
  const grouped = {};
  const groupOrder = [];
  state.groups.forEach(g => {
    grouped[g.Path] = { group: g, sessions: [] };
    groupOrder.push(g.Path);
  });
  state.sessions.forEach(s => {
    if (!grouped[s.GroupPath]) {
      grouped[s.GroupPath] = { group: { Name: s.GroupPath, Path: s.GroupPath }, sessions: [] };
      groupOrder.push(s.GroupPath);
    }
    grouped[s.GroupPath].sessions.push(s);
  });

  groupOrder.forEach(path => {
    const { group, sessions } = grouped[path];
    if (!sessions.length) return;

    const header = document.createElement('div');
    header.className = 'group-header';
    header.textContent = group.Name || group.Path;
    list.appendChild(header);

    sessions.forEach(s => {
      const icon = STATUS_ICONS[s.Status] || STATUS_ICONS.idle;
      const row = document.createElement('div');
      row.className = 'session-row' + (expandedSessions.has(s.ID) ? ' expanded' : '');

      row.innerHTML = `
        <span class="status-dot ${icon.cls}">${icon.char}</span>
        <span class="session-title">${s.HasUncommitted ? '* ' : ''}${s.Title}</span>
        <span class="session-tool">${s.Tool}</span>
      `;

      const detail = document.createElement('div');
      detail.className = 'session-detail' + (expandedSessions.has(s.ID) ? ' open' : '');

      if (expandedSessions.has(s.ID)) {
        const textarea = document.createElement('textarea');
        textarea.className = 'notes-area';
        textarea.value = s.Notes || '';
        textarea.placeholder = 'Notes...';

        const saveBtn = document.createElement('button');
        saveBtn.className = 'save-btn';
        saveBtn.textContent = 'Save notes';
        saveBtn.onclick = (e) => {
          e.stopPropagation();
          saveNotes(s.ID, textarea.value);
        };

        detail.appendChild(textarea);
        detail.appendChild(saveBtn);
        loadEvents(s.ID, detail);
      }

      row.onclick = () => {
        if (expandedSessions.has(s.ID)) {
          expandedSessions.delete(s.ID);
        } else {
          expandedSessions.add(s.ID);
        }
        render();
      };

      list.appendChild(row);
      list.appendChild(detail);
    });
  });
}

document.addEventListener('DOMContentLoaded', () => {
  fetchSessions();
  connectSSE();
});
```

**Step 5: Create static/index.html**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="theme-color" content="#0d1117">
  <title>agent-workspace</title>
  <link rel="manifest" href="/manifest.json">
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <header>
    <h1>agent-workspace</h1>
    <div id="connection-status" title="SSE connection"></div>
  </header>
  <main id="session-list"></main>
  <script src="/app.js"></script>
</body>
</html>
```

**Step 6: Verify it builds**

```
go build ./...
```

Expected: no errors

**Step 7: Commit**

```bash
git add internal/webserver/static.go internal/webserver/static/
git commit -m "feat: add PWA static files with SSE client"
```

---

### Task 9: Wire webserver into main.go and ui/app.go

**Files:**
- Modify: `main.go`
- Modify: `internal/ui/app.go`

**Step 1: No new test needed** -- this is wiring. Verify with `go build ./...`.

**Step 2: Update ui/app.go to accept and use webserver**

Add the webserver to the `App` struct and pass it as the broadcaster:

```go
import (
    // existing imports...
    "github.com/zsprackett/agent-workspace/internal/webserver"
)

type App struct {
    tapp   *tview.Application
    pages  *tview.Pages
    home   *Home
    store  *db.DB
    mgr    *session.Manager
    mon    *monitor.Monitor
    syn    *syncer.Syncer
    cfg    config.Config
    groups []*db.Group
    web    *webserver.Server
}
```

In `NewApp`, after creating the notifier, add:

```go
a.web = webserver.New(store, webserver.Config{
    Enabled: cfg.Webserver.Enabled,
    Port:    cfg.Webserver.Port,
    Host:    cfg.Webserver.Host,
})

a.mon = monitor.New(store, func() {
    a.tapp.QueueUpdateDraw(func() {
        a.refreshHome()
    })
}, notifier, a.web)
```

Also update the notifier creation to pass NtfyURL:

```go
notifier := notify.New(notify.Config{
    Enabled: cfg.Notifications.Enabled,
    Webhook: cfg.Notifications.Webhook,
    NtfyURL: cfg.Notifications.NtfyURL,
})
```

**Step 3: Start the webserver in app.Run()**

Find the `Run()` method in `ui/app.go` and add before `a.mon.Start()`:

```go
if err := a.web.Start(); err != nil {
    // non-fatal -- TUI still works
    fmt.Fprintf(os.Stderr, "warning: webserver: %v\n", err)
}
```

**Step 4: Verify it builds**

```
go build ./...
```

Expected: no errors

**Step 5: Run all tests**

```
go test ./...
```

Expected: PASS

**Step 6: Commit**

```bash
git add main.go internal/ui/app.go
git commit -m "feat: wire webserver into app startup"
```

---

### Task 10: Activity log in TUI right panel

**Files:**
- Modify: `internal/ui/home.go`

**Step 1: Locate the session detail rendering**

Read `internal/ui/home.go` and find the right-column panel that renders session preview/details.

**Step 2: Add event log section**

In the function that renders the right-column session detail, after rendering notes, add a call to fetch and display recent events. Look for where session details are rendered and add:

```go
// After existing detail rendering:
evts, err := a.store.GetSessionEvents(s.ID, 8)
if err == nil && len(evts) > 0 {
    fmt.Fprintf(detail, "\n[::b]Activity[-]\n")
    for _, e := range evts {
        ts := e.Ts.Format("15:04")
        fmt.Fprintf(detail, "[grey]%s[-]  %s\n", ts, e.EventType)
    }
}
```

The exact integration point depends on the current home.go structure -- find the `renderDetail` or equivalent function and add the block after the notes section.

**Step 3: Verify it builds**

```
go build ./...
```

**Step 4: Commit**

```bash
git add internal/ui/home.go
git commit -m "feat: show activity log in TUI session detail panel"
```

---

### Task 11: Update README

**Files:**
- Modify: `README.md`

**Step 1: Add Web UI section after the Notifications section**

```markdown
## Web UI

When agent-workspace starts, it serves a web dashboard at `http://localhost:8080` (configurable). Open it from any browser on the same network -- including iPhone Safari.

To add it to your home screen on iOS: open in Safari, tap Share, then "Add to Home Screen". The app runs full-screen with an icon.

Configuration:

```json
{
  "webserver": {
    "enabled": true,
    "port": 8080,
    "host": "0.0.0.0"
  }
}
```

Set `enabled: false` to disable. Set `host: "127.0.0.1"` to restrict to localhost only.

### Tailscale (access from anywhere)

To access from your phone on a different network:

1. Install [Tailscale](https://tailscale.com) on the Mac running agent-workspace
2. Install Tailscale on your phone
3. Open `http://your-mac.tailnet-name.ts.net:8080`
```

**Step 2: Add ntfy to the Notifications section**

```markdown
### ntfy (mobile push)

Set `notifications.ntfy` to a [ntfy](https://ntfy.sh) topic URL for native push notifications on iOS/Android.

**Self-hosted ntfy** (same machine, same network):
```bash
brew install ntfy
ntfy serve
```
Then set `"ntfy": "http://localhost:8088/agent-workspace"` and subscribe to `agent-workspace` in the ntfy app.

**ntfy.sh public** (works from anywhere -- use a random topic name):
```json
{ "notifications": { "ntfy": "https://ntfy.sh/your-random-uuid-here" } }
```
```

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add web UI, Tailscale, and ntfy sections to README"
```

---

## Smoke Test Checklist

After all tasks complete, verify manually:

1. `make build && ./agent-workspace` -- TUI opens, no errors
2. `curl http://localhost:8080/api/sessions` -- returns JSON
3. Open `http://localhost:8080` in a browser -- session list renders
4. Open browser DevTools → Network → `/events` -- SSE stream visible
5. Start/stop a session in TUI -- browser updates within 1 second
6. Tap a session row in mobile browser -- detail expands with notes editor
7. Edit and save notes in browser -- notes persist (check TUI)
8. Add `"ntfy": "https://ntfy.sh/test-<random>"` to config, subscribe in ntfy app, trigger a waiting state -- push notification arrives
