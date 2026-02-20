package applog_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/applog"
)

func TestDailyRotator_CreatesFileOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 7)
	defer r.Close()

	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	today := time.Now().Format("2006-01-02")
	name := filepath.Join(dir, "agent-workspace-"+today+".log")
	if _, err := os.Stat(name); err != nil {
		t.Errorf("expected log file %q to exist: %v", name, err)
	}
}

func TestDailyRotator_RotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 7)
	defer r.Close()

	// Simulate writing on day 1.
	r.SetNow(func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) })
	if _, err := r.Write([]byte("day1\n")); err != nil {
		t.Fatal(err)
	}

	// Simulate writing on day 2.
	r.SetNow(func() time.Time { return time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) })
	if _, err := r.Write([]byte("day2\n")); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "agent-workspace-*.log"))
	if len(matches) != 2 {
		t.Errorf("expected 2 log files after rotation, got %d", len(matches))
	}
}

func TestDailyRotator_PrunesOldFiles(t *testing.T) {
	dir := t.TempDir()
	r := applog.NewDailyRotator(dir, 3) // keep only 3

	// Write 5 days worth of log files.
	for i := 1; i <= 5; i++ {
		day := i
		r.SetNow(func() time.Time { return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC) })
		if _, err := r.Write([]byte("entry\n")); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()

	matches, _ := filepath.Glob(filepath.Join(dir, "agent-workspace-*.log"))
	if len(matches) != 3 {
		t.Errorf("expected 3 log files after pruning, got %d: %v", len(matches), matches)
	}
	// Oldest file (day 1 and 2) should be gone; day 3, 4, 5 remain.
	for _, name := range matches {
		base := filepath.Base(name)
		if base == "agent-workspace-2026-01-01.log" || base == "agent-workspace-2026-01-02.log" {
			t.Errorf("old file %q should have been pruned", base)
		}
	}
}
