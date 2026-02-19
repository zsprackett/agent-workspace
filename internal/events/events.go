package events

import "github.com/zsprackett/agent-workspace/internal/db"

// Event is a real-time update pushed to web clients.
type Event struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Status    db.SessionStatus `json:"status,omitempty"`
	Title     string          `json:"title,omitempty"`
}

// Broadcaster sends events to connected web clients.
// A nil Broadcaster is safe to use -- Broadcast becomes a no-op.
type Broadcaster interface {
	Broadcast(e Event)
}
