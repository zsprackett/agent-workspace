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

func TestGroupDefaultToolRoundTrip(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	groups := []*db.Group{
		{Path: "ai-work", Name: "AI Work", Expanded: true, SortOrder: 0, DefaultTool: db.ToolOpenCode},
		{Path: "shell-work", Name: "Shell Work", Expanded: true, SortOrder: 1, DefaultTool: db.ToolShell},
		{Path: "no-tool", Name: "No Tool", Expanded: true, SortOrder: 2},
	}
	if err := store.SaveGroups(groups); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.LoadGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[0].DefaultTool != db.ToolOpenCode {
		t.Errorf("DefaultTool[0]: got %q want %q", got[0].DefaultTool, db.ToolOpenCode)
	}
	if got[1].DefaultTool != db.ToolShell {
		t.Errorf("DefaultTool[1]: got %q want %q", got[1].DefaultTool, db.ToolShell)
	}
	if got[2].DefaultTool != "" {
		t.Errorf("DefaultTool[2]: got %q want empty", got[2].DefaultTool)
	}
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSessionEvents(t *testing.T) {
	store := openTestDB(t)

	s := &db.Session{
		ID:          "test-1",
		Title:       "test",
		ProjectPath: "/tmp",
		GroupPath:   "default",
		Tool:        db.ToolClaude,
		Status:      db.StatusIdle,
		TmuxSession: "agws_test",
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatal(err)
	}

	if err := store.InsertSessionEvent("test-1", "created", ""); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}
	if err := store.InsertSessionEvent("test-1", "status_changed", `{"from":"idle","to":"running"}`); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}

	events, err := store.GetSessionEvents("test-1", 50)
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "status_changed" {
		t.Errorf("expected most recent first: got %q", events[0].EventType)
	}
}

func TestAccountCRUD(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	// CreateAccount
	acc, err := store.CreateAccount("alice", "hashed-pw")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.Username != "alice" {
		t.Errorf("expected username alice, got %s", acc.Username)
	}
	if acc.ID == "" {
		t.Error("expected non-empty ID")
	}

	// GetAccountByUsername
	got, err := store.GetAccountByUsername("alice")
	if err != nil {
		t.Fatalf("GetAccountByUsername: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("ID mismatch: %s != %s", got.ID, acc.ID)
	}

	// UpdateAccountPassword
	if err := store.UpdateAccountPassword(acc.ID, "new-hash"); err != nil {
		t.Fatalf("UpdateAccountPassword: %v", err)
	}
	got, _ = store.GetAccountByUsername("alice")
	if got.PasswordHash != "new-hash" {
		t.Error("password not updated")
	}

	// HasAnyAccount
	has, err := store.HasAnyAccount()
	if err != nil {
		t.Fatalf("HasAnyAccount: %v", err)
	}
	if !has {
		t.Error("expected HasAnyAccount to return true")
	}
}

func TestRefreshTokenCRUD(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	acc, _ := store.CreateAccount("bob", "pw")
	exp := time.Now().Add(7 * 24 * time.Hour)

	// CreateRefreshToken
	if err := store.CreateRefreshToken("tok123", acc.ID, exp); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	// GetRefreshToken - valid
	rt, err := store.GetRefreshToken("tok123")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if rt.AccountID != acc.ID {
		t.Errorf("AccountID mismatch")
	}

	// GetRefreshToken - missing
	_, err = store.GetRefreshToken("notexist")
	if err == nil {
		t.Error("expected error for missing token")
	}

	// DeleteRefreshToken
	if err := store.DeleteRefreshToken("tok123"); err != nil {
		t.Fatalf("DeleteRefreshToken: %v", err)
	}
	_, err = store.GetRefreshToken("tok123")
	if err == nil {
		t.Error("expected error after deletion")
	}

	// DeleteRefreshTokensByAccount
	store.CreateRefreshToken("tok-a1", acc.ID, exp)
	store.CreateRefreshToken("tok-a2", acc.ID, exp)
	if err := store.DeleteRefreshTokensByAccount(acc.ID); err != nil {
		t.Fatalf("DeleteRefreshTokensByAccount: %v", err)
	}
	_, err = store.GetRefreshToken("tok-a1")
	if err == nil {
		t.Error("expected tok-a1 deleted")
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
