package syncer_test

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/syncer"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func openDB(t *testing.T) *db.DB {
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

// TestRefresh_NoGroups verifies fetch is never called when there are no groups.
func TestRefresh_NoGroups(t *testing.T) {
	store := openDB(t)
	called := false
	s := syncer.NewWithFetch(store, t.TempDir(), discardLogger(), func(string) error {
		called = true
		return nil
	})
	s.RunOnce()
	if called {
		t.Error("expected fetch not to be called with no groups")
	}
}

// TestRefresh_GroupWithoutRepoURL verifies fetch is skipped for groups with no repo URL.
func TestRefresh_GroupWithoutRepoURL(t *testing.T) {
	store := openDB(t)
	store.SaveGroups([]*db.Group{
		{Path: "my-sessions", Name: "My Sessions", Expanded: true},
	})
	called := false
	s := syncer.NewWithFetch(store, t.TempDir(), discardLogger(), func(string) error {
		called = true
		return nil
	})
	s.RunOnce()
	if called {
		t.Error("expected fetch not to be called for group without repo URL")
	}
}

// TestRefresh_BareRepoNotOnDisk verifies fetch is skipped when bare repo doesn't exist.
func TestRefresh_BareRepoNotOnDisk(t *testing.T) {
	store := openDB(t)
	store.SaveGroups([]*db.Group{
		{Path: "work", Name: "Work", Expanded: true, RepoURL: "https://github.com/owner/myrepo"},
	})
	called := false
	s := syncer.NewWithFetch(store, t.TempDir(), discardLogger(), func(string) error {
		called = true
		return nil
	})
	s.RunOnce()
	if called {
		t.Error("expected fetch not to be called when bare repo is absent")
	}
}

// TestRefresh_FetchCalledForKnownRepo verifies fetch is called when bare repo exists on disk.
func TestRefresh_FetchCalledForKnownRepo(t *testing.T) {
	store := openDB(t)
	reposDir := t.TempDir()

	// Create the expected bare repo directory so os.Stat succeeds.
	bareRepoPath := filepath.Join(reposDir, "github.com", "owner", "myrepo.git")
	if err := os.MkdirAll(bareRepoPath, 0755); err != nil {
		t.Fatal(err)
	}

	store.SaveGroups([]*db.Group{
		{Path: "work", Name: "Work", Expanded: true, RepoURL: "https://github.com/owner/myrepo"},
	})

	var fetchedPath string
	s := syncer.NewWithFetch(store, reposDir, discardLogger(), func(path string) error {
		fetchedPath = path
		return nil
	})
	s.RunOnce()
	if fetchedPath != bareRepoPath {
		t.Errorf("expected fetch for %q, got %q", bareRepoPath, fetchedPath)
	}
}

func TestRefresh_LogsWarnOnFetchError(t *testing.T) {
	store := openDB(t)
	reposDir := t.TempDir()

	bareRepoPath := filepath.Join(reposDir, "github.com", "owner", "myrepo.git")
	if err := os.MkdirAll(bareRepoPath, 0755); err != nil {
		t.Fatal(err)
	}
	store.SaveGroups([]*db.Group{
		{Path: "work", Name: "Work", Expanded: true, RepoURL: "https://github.com/owner/myrepo"},
	})

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := syncer.NewWithFetch(store, reposDir, logger, func(string) error {
		return fmt.Errorf("network error")
	})
	s.RunOnce()

	if !strings.Contains(buf.String(), "network error") {
		t.Errorf("expected warn log with fetch error, got: %q", buf.String())
	}
}
