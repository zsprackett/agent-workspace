package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTool != "claude" {
		t.Errorf("default tool: got %q want claude", cfg.DefaultTool)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"defaultTool":"gemini"}`), 0644)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTool != "gemini" {
		t.Errorf("got %q want gemini", cfg.DefaultTool)
	}
}

func TestLoadWebserverDefaults(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.Webserver.Enabled {
		t.Error("webserver should be enabled by default")
	}
	if cfg.Webserver.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Webserver.Port)
	}
	if cfg.Webserver.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Webserver.Host)
	}
}

func TestLoadNtfyURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"notifications":{"ntfy":"http://localhost:8088/aw"}}`), 0600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.NtfyURL != "http://localhost:8088/aw" {
		t.Errorf("expected ntfy URL, got %q", cfg.Notifications.NtfyURL)
	}
}
