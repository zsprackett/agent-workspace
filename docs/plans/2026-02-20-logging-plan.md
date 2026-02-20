# Logging Infrastructure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Introduce stdlib `slog` with a custom daily-rotating file backend, redirect Go's default `log` package to the same file (capturing `net/http` TLS errors), and thread the logger through `monitor`, `syncer`, and `notify`.

**Architecture:** A new `internal/applog` package owns the `dailyRotator` writer and `Init` function. `Init` is called in `main.go` right after config loads; it redirects both `slog.Default()` and the stdlib `log` package to the rotating file. Each background package (`monitor`, `syncer`, `notify`) receives a `*slog.Logger` via constructor injection.

**Tech Stack:** Go stdlib `log/slog`, `log`, `os`, `sync`, `path/filepath`

---

### Task 1: Config - add LogLevel and LogDir

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestDefaultLogLevel(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level: got %q want info", cfg.LogLevel)
	}
}

func TestDefaultLogDir(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agent-workspace", "logs")
	if cfg.LogDir != want {
		t.Errorf("default log dir: got %q want %q", cfg.LogDir, want)
	}
}

func TestLogLevelOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"logLevel":"debug"}`), 0644)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("got %q want debug", cfg.LogLevel)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: FAIL — `cfg.LogLevel` is empty string.

**Step 3: Add fields to Config**

In `internal/config/config.go`, add to the `Config` struct:

```go
LogLevel string `json:"logLevel"`
LogDir   string `json:"logDir"`
```

In `Defaults()`, add:

```go
LogLevel: "info",
LogDir:   filepath.Join(home, ".agent-workspace", "logs"),
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add LogLevel and LogDir to config"
```

---

### Task 2: applog - dailyRotator

**Files:**
- Create: `internal/applog/applog.go`
- Create: `internal/applog/applog_test.go`

**Step 1: Write the failing tests**

Create `internal/applog/applog_test.go`:

```go
package applog_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/applog"
)

func TestDailyRotator_CreatesFileOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 7)
	defer r.Close()

	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	today := time.Now().Format("2006-01-02")
	name := filepath.Join(dir, "agent-workspace-"+today+".log")
	if _, err := os.Stat(name); err != nil {
		t.Errorf("expected log file %q to exist: %v", name, err)
	}
}

func TestDailyRotator_RotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 7)
	defer r.Close()

	// Simulate writing on day 1.
	r.SetNow(func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) })
	if _, err := r.Write([]byte("day1\n")); err != nil {
		t.Fatal(err)
	}

	// Simulate writing on day 2.
	r.SetNow(func() time.Time { return time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) })
	if _, err := r.Write([]byte("day2\n")); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "agent-workspace-*.log"))
	if len(matches) != 2 {
		t.Errorf("expected 2 log files after rotation, got %d", len(matches))
	}
}

func TestDailyRotator_PrunesOldFiles(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 3) // keep only 3

	// Write 5 days worth of log files.
	for i := 1; i <= 5; i++ {
		day := i
		r.SetNow(func() time.Time { return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC) })
		if _, err := r.Write([]byte("entry\n")); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()

	matches, _ := filepath.Glob(filepath.Join(dir, "agent-workspace-*.log"))
	if len(matches) != 3 {
		t.Errorf("expected 3 log files after pruning, got %d: %v", len(matches), matches)
	}
	// Oldest file (day 1 and 2) should be gone; day 3, 4, 5 remain.
	for _, name := range matches {
		base := filepath.Base(name)
		if base == "agent-workspace-2026-01-01.log" || base == "agent-workspace-2026-01-02.log" {
			t.Errorf("old file %q should have been pruned", base)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/applog/...
```

Expected: FAIL — package `applog` does not exist.

**Step 3: Implement dailyRotator**

Create `internal/applog/applog.go`:

```go
package applog

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DailyRotator is an io.Writer that writes to a date-stamped log file and
// rotates to a new file each calendar day. Old files beyond maxDays are pruned.
type DailyRotator struct {
	mu      sync.Mutex
	dir     string
	date    string
	file    *os.File
	maxDays int
	now     func() time.Time
}

// NewDailyRotator returns a DailyRotator that writes files to dir and keeps
// at most maxDays files.
func NewDailyRotator(dir string, maxDays int) *DailyRotator {
	return &DailyRotator{
		dir:     dir,
		maxDays: maxDays,
		now:     time.Now,
	}
}

// SetNow replaces the time source. Used in tests only.
func (r *DailyRotator) SetNow(fn func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = fn
}

func (r *DailyRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := r.now().Format("2006-01-02")
	if today != r.date {
		if err := r.rotate(today); err != nil {
			return 0, err
		}
	}
	return r.file.Write(p)
}

func (r *DailyRotator) rotate(date string) error {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	name := filepath.Join(r.dir, "agent-workspace-"+date+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.file = f
	r.date = date
	r.prune()
	return nil
}

func (r *DailyRotator) prune() {
	pattern := filepath.Join(r.dir, "agent-workspace-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= r.maxDays {
		return
	}
	sort.Strings(matches)
	for _, f := range matches[:len(matches)-r.maxDays] {
		os.Remove(f)
	}
}

// Close flushes and closes the current log file.
func (r *DailyRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/applog/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/applog/applog.go internal/applog/applog_test.go
git commit -m "feat: add dailyRotator with date-based rotation and pruning"
```

---

### Task 3: applog - Init function

**Files:**
- Modify: `internal/applog/applog.go`
- Modify: `internal/applog/applog_test.go`

**Step 1: Write the failing tests**

Add to `internal/applog/applog_test.go`:

```go
import (
	"io"
	"log"
	"log/slog"
	"strings"
)

func TestInit_CreatesLogDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newlogs")
	_, closer, err := applog.Init(applog.InitConfig{LogDir: dir, LogLevel: "info"})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected log dir %q to be created: %v", dir, err)
	}
}

func TestInit_ParsesLogLevel(t *testing.T) {
	cases := []struct {
		input string
		level slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},   // empty defaults to info
		{"WARN", slog.LevelWarn}, // case insensitive
	}
	for _, tc := range cases {
		got := applog.ParseLevel(tc.input)
		if got != tc.level {
			t.Errorf("ParseLevel(%q): got %v want %v", tc.input, got, tc.level)
		}
	}
}

func TestInit_StdlibLogRedirected(t *testing.T) {
	dir := t.TempDir()
	_, closer, err := applog.Init(applog.InitConfig{LogDir: dir, LogLevel: "info"})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	log.Print("stdlib-log-test-marker")

	today := time.Now().Format("2006-01-02")
	name := filepath.Join(dir, "agent-workspace-"+today+".log")
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "stdlib-log-test-marker") {
		t.Errorf("stdlib log output not found in log file; file contents: %q", string(data))
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/applog/...
```

Expected: FAIL — `applog.Init` not defined.

**Step 3: Add Init and ParseLevel to applog.go**

Add these imports to `internal/applog/applog.go`:

```go
import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)
```

Add these functions to `internal/applog/applog.go`:

```go
// InitConfig holds configuration for Init.
type InitConfig struct {
	LogDir   string
	LogLevel string
}

// Init sets up file-backed structured logging. It redirects both slog.Default
// and the stdlib log package to a daily-rotating file in cfg.LogDir.
// The returned io.Closer must be deferred by the caller.
func Init(cfg InitConfig) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	rotator := NewDailyRotator(cfg.LogDir, 7)
	level := ParseLevel(cfg.LogLevel)
	handler := slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetOutput(rotator)
	log.SetFlags(0)
	return logger, rotator, nil
}

// ParseLevel converts a level string to slog.Level. Defaults to LevelInfo.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/applog/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/applog/applog.go internal/applog/applog_test.go
git commit -m "feat: add applog.Init with slog and stdlib log redirection"
```

---

### Task 4: Wire applog into main.go

**Files:**
- Modify: `main.go`

No new tests needed — this is wiring. Verify with a build.

**Step 1: Update main.go**

Replace the `main` function body (after `cfg` is loaded and before `dbPath`) with:

```go
	logger, logCloser, err := applog.Init(applog.InitConfig{
		LogDir:   cfg.LogDir,
		LogLevel: cfg.LogLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not init log file: %v\n", err)
		logger = slog.Default() // falls back to default (stderr)
	} else {
		defer logCloser.Close()
	}
```

Add `"log/slog"` and `"github.com/zsprackett/agent-workspace/internal/applog"` to imports.

Pass `logger` to `ui.NewApp`:

```go
	app := ui.NewApp(store, cfg, logger)
```

**Step 2: Build to verify**

```bash
make build
```

Expected: compile error on `ui.NewApp` — that's expected and will be fixed in Task 8.

For now, just verify the `applog.Init` call compiles in isolation:

```bash
go build ./internal/applog/...
go build ./internal/config/...
```

Expected: PASS

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire applog.Init into main"
```

---

### Task 5: Update syncer to accept logger

**Files:**
- Modify: `internal/syncer/syncer.go`
- Modify: `internal/syncer/syncer_test.go`

**Step 1: Write the failing test**

Add to `internal/syncer/syncer_test.go`:

```go
import (
	"log/slog"
	"strings"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRefresh_LogsWarnOnFetchError(t *testing.T) {
	store := openDB(t)
	reposDir := t.TempDir()

	bareRepoPath := filepath.Join(reposDir, "github.com", "owner", "myrepo.git")
	if err := os.MkdirAll(bareRepoPath, 0755); err != nil {
		t.Fatal(err)
	}
	store.SaveGroups([]*db.Group{
		{Path: "work", Name: "Work", Expanded: true, RepoURL: "https://github.com/owner/myrepo"},
	})

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := syncer.NewWithFetch(store, reposDir, logger, func(string) error {
		return fmt.Errorf("network error")
	})
	s.RunOnce()

	if !strings.Contains(buf.String(), "network error") {
		t.Errorf("expected warn log with fetch error, got: %q", buf.String())
	}
}
```

Also update all existing `NewWithFetch` calls in the test file to pass `discardLogger()` as the third argument (before the fetch func).

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/syncer/...
```

Expected: FAIL — `NewWithFetch` wrong number of arguments.

**Step 3: Update syncer.go**

Add `"log/slog"` to imports. Remove `"log"` from imports.

Update `Syncer` struct:

```go
type Syncer struct {
	db       *db.DB
	reposDir string
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
	fetch    func(repoDir string) error
	logger   *slog.Logger
}
```

Update `New`:

```go
func New(store *db.DB, reposDir string, logger *slog.Logger) *Syncer {
	return &Syncer{
		db:       store,
		reposDir: reposDir,
		interval: 2 * time.Minute,
		stop:     make(chan struct{}),
		fetch:    git.FetchBare,
		logger:   logger,
	}
}
```

Update `NewWithFetch`:

```go
func NewWithFetch(store *db.DB, reposDir string, logger *slog.Logger, fetch func(repoDir string) error) *Syncer {
	s := New(store, reposDir, logger)
	s.fetch = fetch
	return s
}
```

Replace `log.Printf("syncer: fetch %s: %v", path, err)` with:

```go
s.logger.Warn("syncer: fetch failed", "repo", path, "err", err)
```

**Step 4: Update syncer_test.go existing calls**

Update all four existing `syncer.NewWithFetch(store, ...)` calls to insert `discardLogger()` as the third argument. Add `"io"` and `"fmt"` to imports if not present.

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/syncer/...
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/syncer/syncer.go internal/syncer/syncer_test.go
git commit -m "feat: inject *slog.Logger into syncer"
```

---

### Task 6: Update notify to accept logger

**Files:**
- Modify: `internal/notify/notify.go`

No existing tests for notify. Add a minimal one.

**Step 1: Write the failing test**

Create `internal/notify/notify_test.go`:

```go
package notify_test

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

func TestNotify_WebhookErrorLogged(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Invalid URL forces a POST error.
	n := notify.New(notify.Config{Enabled: true, Webhook: "http://127.0.0.1:1"}, logger)
	n.Notify(db.Session{Title: "test", Tool: "claude"})

	if !strings.Contains(buf.String(), "webhook") {
		t.Errorf("expected warn log mentioning webhook, got: %q", buf.String())
	}
}

func TestNotify_DisabledNoOp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	n := notify.New(notify.Config{Enabled: false}, logger)
	// Must not panic.
	n.Notify(db.Session{Title: "test", Tool: "claude"})
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/notify/...
```

Expected: FAIL — `notify.New` wrong number of arguments.

**Step 3: Update notify.go**

Add `"log/slog"` to imports.

Update `Notifier`:

```go
type Notifier struct {
	cfg    Config
	logger *slog.Logger
}
```

Update `New`:

```go
func New(cfg Config, logger *slog.Logger) *Notifier {
	return &Notifier{cfg: cfg, logger: logger}
}
```

In `sendWebhook`, capture and log the error:

```go
func (n *Notifier) sendWebhook(s db.Session) {
	payload := webhookPayload{
		Session:   s.Title,
		Tool:      string(s.Tool),
		Group:     s.GroupPath,
		Status:    string(s.Status),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(n.cfg.Webhook, "application/json", bytes.NewReader(data))
	if err != nil {
		n.logger.Warn("notify: webhook POST failed", "err", err)
		return
	}
	resp.Body.Close()
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/notify/...
```

Expected: PASS (the webhook test may be slow due to TCP dial timeout; it should still pass).

**Step 5: Commit**

```bash
git add internal/notify/notify.go internal/notify/notify_test.go
git commit -m "feat: inject *slog.Logger into notify, log webhook errors"
```

---

### Task 7: Update monitor to accept logger

**Files:**
- Modify: `internal/monitor/monitor.go`

**Step 1: Update monitor.go**

Add `"log/slog"` to imports.

Update `Monitor` struct:

```go
type Monitor struct {
	db         *db.DB
	onUpdate   OnUpdate
	notifier   *notify.Notifier
	prevStatus map[string]db.SessionStatus
	interval   time.Duration
	stop       chan struct{}
	wg         sync.WaitGroup
	logger     *slog.Logger
}
```

Update `New`:

```go
func New(store *db.DB, onUpdate OnUpdate, notifier *notify.Notifier, logger *slog.Logger) *Monitor {
	return &Monitor{
		db:         store,
		onUpdate:   onUpdate,
		notifier:   notifier,
		prevStatus: make(map[string]db.SessionStatus),
		interval:   500 * time.Millisecond,
		stop:       make(chan struct{}),
		logger:     logger,
	}
}
```

In `refresh`, add a debug log when a session status changes:

```go
if newStatus != s.Status {
	prev := m.prevStatus[s.ID]
	m.db.WriteStatus(s.ID, newStatus, s.Tool)
	m.logger.Debug("monitor: status changed",
		"session", s.Title,
		"from", string(s.Status),
		"to", string(newStatus),
	)
	changed = true
	if newStatus == db.StatusWaiting && prev != db.StatusWaiting {
		s.Status = newStatus
		m.notifier.Notify(*s)
	}
}
```

**Step 2: Build to verify no compile errors**

```bash
go build ./internal/monitor/...
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/monitor/monitor.go
git commit -m "feat: inject *slog.Logger into monitor, debug-log status changes"
```

---

### Task 8: Update ui/app.go and wire everything together

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `main.go`

**Step 1: Update ui/app.go**

Add `"log/slog"` to imports.

Add `logger *slog.Logger` field to `App`:

```go
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
	logger *slog.Logger
}
```

Update `NewApp` signature:

```go
func NewApp(store *db.DB, cfg config.Config, logger *slog.Logger) *App {
```

In `NewApp`, store the logger and pass it to constructors:

```go
a := &App{
	store:  store,
	cfg:    cfg,
	mgr:    session.NewManager(store),
	logger: logger,
}
```

Update `notify.New` call:

```go
notifier := notify.New(notify.Config{
	Enabled: cfg.Notifications.Enabled,
	Webhook: cfg.Notifications.Webhook,
}, logger)
```

Update `monitor.New` call:

```go
a.mon = monitor.New(store, func() {
	a.tapp.QueueUpdateDraw(func() {
		a.refreshHome()
	})
}, notifier, logger)
```

Update `syncer.New` call:

```go
a.syn = syncer.New(store, cfg.ReposDir, logger)
```

**Step 2: Update main.go**

The `ui.NewApp` call already has `logger` passed from Task 4. Verify it compiles now.

**Step 3: Build the full binary**

```bash
make build
```

Expected: PASS — clean compile.

**Step 4: Run all tests**

```bash
make test
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/ui/app.go main.go
git commit -m "feat: thread *slog.Logger through ui.App, completing logging wiring"
```

---

### Task 9: Final smoke test

**Step 1: Run the binary and verify no console noise**

```bash
./agent-workspace
```

Start the TUI and leave it running for 30 seconds. The TLS handshake errors and any syncer fetch errors should no longer appear in the terminal.

**Step 2: Verify the log file exists**

```bash
ls -la ~/.agent-workspace/logs/
cat ~/.agent-workspace/logs/agent-workspace-$(date +%Y-%m-%d).log
```

Expected: log file present with entries.

**Step 3: Verify log level config works**

Add `"logLevel": "debug"` to `~/.agent-workspace/config.json`, restart, and verify debug entries appear in the log file.
