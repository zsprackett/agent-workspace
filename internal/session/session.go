package session

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/git"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

var adjectives = []string{
	"swift", "bright", "calm", "deep", "eager", "fair", "gentle", "happy",
	"keen", "light", "mild", "noble", "proud", "quick", "rich", "safe",
	"true", "vivid", "warm", "wise", "bold", "cool", "dark", "fast",
}

var nouns = []string{
	"fox", "owl", "wolf", "bear", "hawk", "lion", "deer", "crow",
	"dove", "seal", "swan", "hare", "lynx", "moth", "newt", "orca",
	"pike", "rook", "toad", "vole", "wren", "yak", "bass", "crab",
}

func GenerateTitle() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	return fmt.Sprintf("%s-%s", adj, noun)
}

type CreateOptions struct {
	Title          string
	ProjectPath    string
	GroupPath      string
	Tool           db.Tool
	Command        string
	WorktreePath   string
	WorktreeRepo   string
	WorktreeBranch string
	RepoURL        string
}

type Manager struct {
	db *db.DB
}

func NewManager(store *db.DB) *Manager {
	return &Manager{db: store}
}

func (m *Manager) Create(opts CreateOptions) (*db.Session, error) {
	title := opts.Title
	if title == "" {
		title = GenerateTitle()
	}
	groupPath := opts.GroupPath
	if groupPath == "" {
		groupPath = "my-sessions"
	}
	command := opts.Command
	if command == "" {
		command = db.ToolCommand(opts.Tool, "")
	}

	tmuxName := tmux.GenerateSessionName(title)
	if err := tmux.CreateSession(tmux.CreateOptions{
		Name:    tmuxName,
		Command: command,
		Cwd:     opts.ProjectPath,
	}); err != nil {
		return nil, fmt.Errorf("create tmux session: %w", err)
	}

	sessions, _ := m.db.LoadSessions()
	now := time.Now()
	s := &db.Session{
		ID:             uuid.NewString(),
		Title:          title,
		ProjectPath:    opts.ProjectPath,
		GroupPath:      groupPath,
		SortOrder:      len(sessions),
		Command:        command,
		Tool:           opts.Tool,
		Status:         db.StatusRunning,
		TmuxSession:    tmuxName,
		CreatedAt:      now,
		LastAccessed:   now,
		WorktreePath:   opts.WorktreePath,
		WorktreeRepo:   opts.WorktreeRepo,
		WorktreeBranch: opts.WorktreeBranch,
		RepoURL:        opts.RepoURL,
	}

	if err := m.db.SaveSession(s); err != nil {
		return nil, err
	}
	m.db.Touch()
	return s, nil
}

func (m *Manager) Delete(id string) error {
	s, err := m.db.GetSession(id)
	if err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	if s.TmuxSession != "" {
		tmux.KillSession(s.TmuxSession)
	}
	if err := m.db.DeleteSession(id); err != nil {
		return err
	}
	return m.db.Touch()
}

func (m *Manager) Stop(id string) error {
	s, err := m.db.GetSession(id)
	if err != nil || s == nil {
		return err
	}
	if s.TmuxSession != "" {
		tmux.KillSession(s.TmuxSession)
	}
	if err := m.db.WriteStatus(id, db.StatusStopped, s.Tool); err != nil {
		return err
	}
	return m.db.Touch()
}

func (m *Manager) Restart(id string) error {
	s, err := m.db.GetSession(id)
	if err != nil || s == nil {
		return fmt.Errorf("session not found: %s", id)
	}
	if s.TmuxSession != "" {
		tmux.KillSession(s.TmuxSession)
	}
	newName := tmux.GenerateSessionName(s.Title)
	if err := tmux.CreateSession(tmux.CreateOptions{
		Name:    newName,
		Command: s.Command,
		Cwd:     s.ProjectPath,
	}); err != nil {
		return err
	}
	s.TmuxSession = newName
	s.Status = db.StatusRunning
	s.LastAccessed = time.Now()
	if err := m.db.SaveSession(s); err != nil {
		return err
	}
	return m.db.Touch()
}

func (m *Manager) Rename(id, title string) error {
	if err := m.db.UpdateSessionField(id, "title", title); err != nil {
		return err
	}
	return m.db.Touch()
}

type UpdateOptions struct {
	Title       string
	Tool        db.Tool
	ProjectPath string
	GroupPath   string
}

func (m *Manager) Update(id string, opts UpdateOptions) error {
	s, err := m.db.GetSession(id)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("session not found: %s", id)
	}
	s.Title = opts.Title
	s.Tool = opts.Tool
	s.Command = db.ToolCommand(opts.Tool, "")
	s.ProjectPath = opts.ProjectPath
	s.GroupPath = opts.GroupPath
	if err := m.db.SaveSession(s); err != nil {
		return err
	}
	return m.db.Touch()
}

func (m *Manager) MoveToGroup(id, groupPath string) error {
	if err := m.db.UpdateSessionField(id, "group_path", groupPath); err != nil {
		return err
	}
	return m.db.Touch()
}

func (m *Manager) Attach(id string) error {
	s, err := m.db.GetSession(id)
	if err != nil || s == nil {
		return fmt.Errorf("session not found: %s", id)
	}
	if s.TmuxSession == "" {
		return fmt.Errorf("session has no tmux session")
	}
	getStats := func() (running, waiting, total int) {
		all, _ := m.db.LoadSessions()
		for _, sess := range all {
			switch sess.Status {
			case db.StatusRunning:
				running++
			case db.StatusWaiting:
				waiting++
			}
		}
		return running, waiting, len(all)
	}
	if err := tmux.AttachSession(s.TmuxSession, s.Title, getStats); err != nil {
		return err
	}
	if s.WorktreePath != "" {
		if dirty, err := git.IsWorktreeDirty(s.WorktreePath); err == nil {
			m.db.UpdateSessionDirty(s.ID, dirty)
		}
	}
	return nil
}

func (m *Manager) List() ([]*db.Session, error) {
	return m.db.LoadSessions()
}

func (m *Manager) Get(id string) (*db.Session, error) {
	return m.db.GetSession(id)
}

func (m *Manager) Acknowledge(id string) error {
	if err := m.db.SetAcknowledged(id, true); err != nil {
		return err
	}
	return m.db.Touch()
}
