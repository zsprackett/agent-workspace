package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type WorktreeConfig struct {
	DefaultBaseBranch string `json:"defaultBaseBranch"`
}

type NotificationsConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	NtfyURL string `json:"ntfy"`
}

type WebserverConfig struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Host    string `json:"host"`
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

func ReposDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-workspace", "repos")
}

func WorktreesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-workspace", "worktrees")
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-workspace", "config.json")
}

func DBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-workspace", "state.db")
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
