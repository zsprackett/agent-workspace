package config

import (
	"crypto/rand"
	"encoding/hex"
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

type TLSConfig struct {
	Mode     string `json:"mode"`     // "self-signed", "autocert", "manual", or "" (disabled)
	Domain   string `json:"domain"`   // required for autocert
	CertFile string `json:"certFile"` // required for manual
	KeyFile  string `json:"keyFile"`  // required for manual
	CacheDir string `json:"cacheDir"` // for autocert and self-signed; defaults to ~/.agent-workspace/certs
}

type AuthConfig struct {
	JWTSecret       string `json:"jwtSecret"`
	RefreshTokenTTL string `json:"refreshTokenTTL"` // e.g. "168h", defaults to 7 days
}

type WebserverConfig struct {
	Enabled bool       `json:"enabled"`
	Port    int        `json:"port"`
	Host    string     `json:"host"`
	TLS     TLSConfig  `json:"tls"`
	Auth    AuthConfig `json:"auth"`
}

type Config struct {
	DefaultTool   string              `json:"defaultTool"`
	DefaultGroup  string              `json:"defaultGroup"`
	Worktree      WorktreeConfig      `json:"worktree"`
	ReposDir      string              `json:"reposDir"`
	WorktreesDir  string              `json:"worktreesDir"`
	Notifications NotificationsConfig `json:"notifications"`
	Webserver     WebserverConfig     `json:"webserver"`
	LogLevel      string              `json:"logLevel"`
	LogDir        string              `json:"logDir"`
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
		LogLevel: "info",
		LogDir:   filepath.Join(home, ".agent-workspace", "logs"),
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

// EnsureJWTSecret generates and saves a JWT secret if one is not already set.
// It writes the updated config back to path.
func EnsureJWTSecret(path string, cfg *Config) error {
	if cfg.Webserver.Auth.JWTSecret != "" {
		return nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	cfg.Webserver.Auth.JWTSecret = hex.EncodeToString(b)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
