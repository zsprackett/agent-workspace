package monitor_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
	"github.com/zsprackett/agent-workspace/internal/monitor"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type captureBroadcaster struct {
	events []events.Event
}

func (c *captureBroadcaster) Broadcast(e events.Event) {
	c.events = append(c.events, e)
}

func TestMonitorSkipsCreatingSession(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	now := time.Now()
	s := &db.Session{
		ID:           "creating-id",
		Title:        "pending-fox",
		GroupPath:    "my-sessions",
		Tool:         db.ToolClaude,
		Status:       db.StatusCreating,
		TmuxSession:  "", // no tmux session yet
		CreatedAt:    now,
		LastAccessed: now,
	}
	store.SaveSession(s)

	updateCalled := false
	notifier := notify.New(notify.Config{}, discardLogger())
	mon := monitor.New(store, func() { updateCalled = true }, notifier, nil, discardLogger())

	mon.Start()
	time.Sleep(600 * time.Millisecond) // one tick
	mon.Stop()

	got, _ := store.GetSession("creating-id")
	if got.Status != db.StatusCreating {
		t.Errorf("expected StatusCreating, got %q", got.Status)
	}
	if updateCalled {
		t.Error("expected no update callback for creating session")
	}
}

func TestMonitorAcceptsBroadcaster(t *testing.T) {
	broadcaster := &captureBroadcaster{}
	notifier := notify.New(notify.Config{}, discardLogger())

	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	mon := monitor.New(store, func() {}, notifier, broadcaster, discardLogger())
	if mon == nil {
		t.Fatal("expected non-nil monitor")
	}
}
