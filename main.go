package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/zsprackett/agent-workspace/internal/applog"
	"github.com/zsprackett/agent-workspace/internal/config"
	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/tmux"
	"github.com/zsprackett/agent-workspace/internal/ui"
	"github.com/zsprackett/agent-workspace/internal/ui/menucmd"
	"github.com/zsprackett/agent-workspace/internal/ui/notescmd"
)

func openDB() (*db.DB, error) {
	dbPath := config.DBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, err
	}
	store, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func main() {
	// notes subcommand: invoked from within a tmux session via display-popup
	if len(os.Args) == 3 && os.Args[1] == "notes" {
		if err := notescmd.Run(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// menu subcommand: invoked via the Ctrl+\ leader key inside a session.
	// panePath is not passed as an argument; tmux sets display-popup CWD to the
	// active pane's directory, so menucmd reads it via os.Getwd().
	if len(os.Args) == 3 && os.Args[1] == "menu" {
		if err := menucmd.Run(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) >= 3 && os.Args[1] == "adduser" {
		username := os.Args[2]
		fmt.Printf("Password for %s: ", username)
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		hash, err := bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		store, err := openDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer store.Close()
		if _, err := store.CreateAccount(username, string(hash)); err != nil {
			fmt.Fprintf(os.Stderr, "error creating account: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Account created: %s\n", username)
		return
	}

	if len(os.Args) >= 3 && os.Args[1] == "passwd" {
		username := os.Args[2]
		fmt.Printf("New password for %s: ", username)
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		hash, err := bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		store, err := openDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer store.Close()
		acc, err := store.GetAccountByUsername(username)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: user not found: %v\n", err)
			os.Exit(1)
		}
		store.UpdateAccountPassword(acc.ID, string(hash))
		store.DeleteRefreshTokensByAccount(acc.ID)
		fmt.Printf("Password updated: %s (all sessions invalidated)\n", username)
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

	if err := config.EnsureJWTSecret(config.DefaultPath(), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not persist JWT secret: %v\n", err)
	}

	logger, logCloser, err := applog.Init(applog.InitConfig{
		LogDir:   cfg.LogDir,
		LogLevel: cfg.LogLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not init log file: %v\n", err)
		logger = slog.Default() // falls back to default (stderr)
	} else {
		defer logCloser.Close()
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

	app := ui.NewApp(store, cfg, logger)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
