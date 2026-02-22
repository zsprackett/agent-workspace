package monitor

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
	"github.com/zsprackett/agent-workspace/internal/notify"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

type OnUpdate func()

type Monitor struct {
	db            *db.DB
	onUpdate      OnUpdate
	notifier      *notify.Notifier
	broadcaster   events.Broadcaster
	prevStatus    map[string]db.SessionStatus
	pendingStatus map[string]db.SessionStatus
	interval      time.Duration
	stop          chan struct{}
	wg            sync.WaitGroup
	logger        *slog.Logger
}

func New(store *db.DB, onUpdate OnUpdate, notifier *notify.Notifier, broadcaster events.Broadcaster, logger *slog.Logger) *Monitor {
	return &Monitor{
		db:            store,
		onUpdate:      onUpdate,
		notifier:      notifier,
		broadcaster:   broadcaster,
		prevStatus:    make(map[string]db.SessionStatus),
		pendingStatus: make(map[string]db.SessionStatus),
		interval:      500 * time.Millisecond,
		stop:          make(chan struct{}),
		logger:        logger,
	}
}

func (m *Monitor) Start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
				m.refresh()
			}
		}
	}()
}

func (m *Monitor) Stop() {
	close(m.stop)
	m.wg.Wait()
}

func (m *Monitor) refresh() {
	sessions, err := m.db.LoadSessions()
	if err != nil || len(sessions) == 0 {
		return
	}

	tmuxSessions, err := tmux.ListSessions()
	if err != nil {
		return
	}

	changed := false
	for _, s := range sessions {
		// Skip sessions that are being created or deleted - they have no tmux session yet.
		if s.Status == db.StatusCreating || s.Status == db.StatusDeleting {
			continue
		}
		if s.TmuxSession == "" {
			continue
		}
		if !tmux.SessionExists(s.TmuxSession, tmuxSessions) {
			m.db.WriteStatus(s.ID, db.StatusStopped, s.Tool)
			changed = true
			continue
		}

		output, err := tmux.CapturePane(s.TmuxSession, tmux.CaptureOptions{
			StartLine: -100,
			Join:      true,
		})
		if err != nil {
			continue
		}

		status := tmux.ParseToolStatus(output, string(s.Tool))
		isActive := tmux.IsSessionActive(s.TmuxSession, tmuxSessions, 2)
		isWaitingForInput := tmux.IsPaneWaitingForInput(s.TmuxSession)

		var newStatus db.SessionStatus
		switch {
		case status.IsWaiting || isWaitingForInput:
			newStatus = db.StatusWaiting
		case status.IsBusy || isActive:
			newStatus = db.StatusRunning
		case status.HasError:
			newStatus = db.StatusError
		default:
			newStatus = db.StatusIdle
		}

		if newStatus != s.Status {
			if m.pendingStatus[s.ID] == newStatus {
				// Stable for 2 consecutive ticks - commit the change.
				prev := m.prevStatus[s.ID]
				m.db.WriteStatus(s.ID, newStatus, s.Tool)
				m.logger.Debug("monitor: status changed",
					"session", s.Title,
					"from", string(s.Status),
					"to", string(newStatus),
				)
				changed = true
				detail, _ := json.Marshal(map[string]string{"from": string(s.Status), "to": string(newStatus)})
				m.db.InsertSessionEvent(s.ID, "status_changed", string(detail))
				m.broadcast(events.Event{
					Type:      "status_changed",
					SessionID: s.ID,
					Status:    newStatus,
					Title:     s.Title,
				})
				if newStatus == db.StatusWaiting && prev != db.StatusWaiting {
					s.Status = newStatus
					m.notifier.Notify(*s)
				}
				m.prevStatus[s.ID] = newStatus
				delete(m.pendingStatus, s.ID)
			} else {
				// First sighting of this new status - wait for confirmation.
				m.pendingStatus[s.ID] = newStatus
			}
		} else {
			// Status is stable - clear any pending candidate.
			delete(m.pendingStatus, s.ID)
			m.prevStatus[s.ID] = s.Status
		}
	}

	if changed {
		m.db.Touch()
		if m.onUpdate != nil {
			m.onUpdate()
		}
	}
}

func (m *Monitor) broadcast(e events.Event) {
	if m.broadcaster != nil {
		m.broadcaster.Broadcast(e)
	}
}
