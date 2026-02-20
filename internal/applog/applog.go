package applog

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DailyRotator is an io.Writer that writes to a date-stamped log file and
// rotates to a new file each calendar day. Old files beyond maxDays are pruned.
type DailyRotator struct {
	mu      sync.Mutex
	dir     string
	date    string
	file    *os.File
	maxDays int
	now     func() time.Time
}

// NewDailyRotator returns a DailyRotator that writes files to dir and keeps
// at most maxDays files.
func NewDailyRotator(dir string, maxDays int) *DailyRotator {
	return &DailyRotator{
		dir:     dir,
		maxDays: maxDays,
		now:     time.Now,
	}
}

// SetNow replaces the time source. Used in tests only.
func (r *DailyRotator) SetNow(fn func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = fn
}

func (r *DailyRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := r.now().Format("2006-01-02")
	if today != r.date {
		if err := r.rotate(today); err != nil {
			return 0, err
		}
	}
	return r.file.Write(p)
}

func (r *DailyRotator) rotate(date string) error {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	name := filepath.Join(r.dir, "agent-workspace-"+date+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.file = f
	r.date = date
	r.prune()
	return nil
}

func (r *DailyRotator) prune() {
	pattern := filepath.Join(r.dir, "agent-workspace-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= r.maxDays {
		return
	}
	sort.Strings(matches)
	for _, f := range matches[:len(matches)-r.maxDays] {
		os.Remove(f)
	}
}

// Close flushes and closes the current log file.
func (r *DailyRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}
