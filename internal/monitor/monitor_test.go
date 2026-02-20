package monitor_test

import (
	"io"
	"log/slog"
	"testing"

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
