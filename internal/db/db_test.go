package db_test

import (
	"testing"
	"time"
	"github.com/zsprackett/agent-workspace/internal/db"
)

func TestMigrate(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
}

func TestSessionCRUD(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "test-id",
		Title:        "swift-fox",
		ProjectPath:  "/tmp/myproject",
		GroupPath:    "my-sessions",
		Command:      "claude",
		Tool:         db.ToolClaude,
		Status:       db.StatusRunning,
		TmuxSession:  "agws_swift-fox-abc",
		CreatedAt:    now,
		LastAccessed: now,
	}

	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.GetSession("test-id")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "swift-fox" {
		t.Errorf("title: got %q want %q", got.Title, "swift-fox")
	}

	sessions, err := store.LoadSessions()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if err := store.WriteStatus("test-id", db.StatusIdle, db.ToolClaude); err != nil {
		t.Fatalf("write status: %v", err)
	}

	got, _ = store.GetSession("test-id")
	if got.Status != db.StatusIdle {
		t.Errorf("status: got %q want idle", got.Status)
	}

	if err := store.DeleteSession("test-id"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	sessions, _ = store.LoadSessions()
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(sessions))
	}
}

func TestGroupCRUD(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	groups := []*db.Group{
		{Path: "my-sessions", Name: "My Sessions", Expanded: true, SortOrder: 0},
		{Path: "work", Name: "Work", Expanded: false, SortOrder: 1},
	}
	if err := store.SaveGroups(groups); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.LoadGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "My Sessions" {
		t.Errorf("name: %q", got[0].Name)
	}

	if err := store.DeleteGroup("work"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = store.LoadGroups()
	if len(got) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(got))
	}
}

func TestMetadata(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	store.Touch()
	ts := store.LastModified()
	if ts == 0 {
		t.Error("expected non-zero last modified")
	}
}

func TestGroupPreLaunchCommand(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	groups := []*db.Group{
		{
			Path:             "work",
			Name:             "Work",
			Expanded:         true,
			SortOrder:        0,
			PreLaunchCommand: "/usr/local/bin/setup.sh",
		},
	}
	if err := store.SaveGroups(groups); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.LoadGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].PreLaunchCommand != "/usr/local/bin/setup.sh" {
		t.Errorf("pre_launch_command: got %q want %q", got[0].PreLaunchCommand, "/usr/local/bin/setup.sh")
	}
}

func TestRepoURLRoundTrip(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "repo-url-test",
		Title:        "bold-bear",
		ProjectPath:  "/tmp/wt",
		GroupPath:    "my-sessions",
		Command:      "claude",
		Tool:         db.ToolClaude,
		Status:       db.StatusRunning,
		TmuxSession:  "agws_bold-bear-xyz",
		CreatedAt:    now,
		LastAccessed: now,
		RepoURL:      "https://github.com/owner/myrepo",
	}

	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.GetSession("repo-url-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RepoURL != "https://github.com/owner/myrepo" {
		t.Errorf("repo_url: got %q want %q", got.RepoURL, "https://github.com/owner/myrepo")
	}
}
