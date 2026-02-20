package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
)

// Config holds notification settings.
type Config struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	NtfyURL string `json:"ntfy"`
}

// Notifier fires system notifications and optional webhook POSTs.
type Notifier struct {
	cfg Config
}

// New returns a Notifier with the given config.
func New(cfg Config) *Notifier {
	return &Notifier{cfg: cfg}
}

// Notify sends a system notification and optional webhook POST for a session
// that has transitioned to waiting.
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

func (n *Notifier) sendSystemNotification(msg string) {
	script := fmt.Sprintf(
		`display notification %q with title "agent-workspace"`,
		msg,
	)
	exec.Command("osascript", "-e", script).Run()
}

type webhookPayload struct {
	Session   string `json:"session"`
	Tool      string `json:"tool"`
	Group     string `json:"group"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

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
	client.Post(n.cfg.Webhook, "application/json", bytes.NewReader(data))
}

type ntfyPayload struct {
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
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
