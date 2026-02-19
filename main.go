package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsprackett/agent-workspace/internal/config"
	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/tmux"
	"github.com/zsprackett/agent-workspace/internal/ui"
	"github.com/zsprackett/agent-workspace/internal/ui/notescmd"
)

func main() {
	// notes subcommand: invoked from within a tmux session via display-popup
	if len(os.Args) == 3 && os.Args[1] == "notes" {
		if err := notescmd.Run(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if !tmux.IsAvailable() {
		fmt.Fprintln(os.Stderr, "error: tmux is required but not found in PATH")
		os.Exit(1)
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config: %v\n", err)
		cfg = config.Defaults()
	}

	dbPath := config.DBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not create data directory: %v\n", err)
		os.Exit(1)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: database migration failed: %v\n", err)
		os.Exit(1)
	}

	app := ui.NewApp(store, cfg)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
