package db

import "time"

type SessionStatus string

const (
	StatusRunning SessionStatus = "running"
	StatusWaiting SessionStatus = "waiting"
	StatusIdle    SessionStatus = "idle"
	StatusStopped SessionStatus = "stopped"
	StatusError   SessionStatus = "error"
)

type Tool string

const (
	ToolClaude   Tool = "claude"
	ToolOpenCode Tool = "opencode"
	ToolGemini   Tool = "gemini"
	ToolCodex    Tool = "codex"
	ToolCustom   Tool = "custom"
	ToolShell    Tool = "shell"
)

func ToolCommand(t Tool, custom string) string {
	switch t {
	case ToolClaude:
		return "claude"
	case ToolOpenCode:
		return "opencode"
	case ToolGemini:
		return "gemini"
	case ToolCodex:
		return "codex"
	case ToolCustom:
		if custom != "" {
			return custom
		}
		return "/bin/bash"
	default:
		return "/bin/bash"
	}
}

type Session struct {
	ID              string
	Title           string
	ProjectPath     string
	GroupPath       string
	SortOrder       int
	Command         string
	Tool            Tool
	Status          SessionStatus
	TmuxSession     string
	CreatedAt       time.Time
	LastAccessed    time.Time
	ParentSessionID string
	WorktreePath    string
	WorktreeRepo    string
	WorktreeBranch  string
	Acknowledged    bool
	RepoURL         string
}

type Group struct {
	Path        string
	Name        string
	Expanded    bool
	SortOrder   int
	DefaultPath string
	RepoURL     string
}

type StatusUpdate struct {
	SessionID    string
	Status       SessionStatus
	Tool         Tool
	Acknowledged bool
}
