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
