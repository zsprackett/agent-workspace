package syncer

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/git"
)

type Syncer struct {
	db       *db.DB
	reposDir string
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
	fetch    func(repoDir string) error
	logger   *slog.Logger
}

func New(store *db.DB, reposDir string, logger *slog.Logger) *Syncer {
	return &Syncer{
		db:       store,
		reposDir: reposDir,
		interval: 2 * time.Minute,
		stop:     make(chan struct{}),
		fetch:    git.FetchBare,
		logger:   logger,
	}
}

// NewWithFetch creates a Syncer with an injectable fetch function. Used in tests.
func NewWithFetch(store *db.DB, reposDir string, logger *slog.Logger, fetch func(repoDir string) error) *Syncer {
	s := New(store, reposDir, logger)
	s.fetch = fetch
	return s
}

func (s *Syncer) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.refresh()
			}
		}
	}()
}

func (s *Syncer) Stop() {
	close(s.stop)
	s.wg.Wait()
}

// RunOnce runs a single refresh cycle synchronously. Used in tests.
func (s *Syncer) RunOnce() {
	s.refresh()
}

func (s *Syncer) refresh() {
	groups, err := s.db.LoadGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.RepoURL == "" {
			continue
		}
		host, owner, repo, err := git.ParseRepoURL(g.RepoURL)
		if err != nil {
			continue
		}
		path := git.BareRepoPath(s.reposDir, host, owner, repo)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := s.fetch(path); err != nil {
			s.logger.Warn("syncer: fetch failed", "repo", path, "err", err)
		}
		s.updateDirtyStatus(g.Path)
	}
}

func (s *Syncer) updateDirtyStatus(groupPath string) {
	sessions, err := s.db.LoadSessionsByGroupPath(groupPath)
	if err != nil {
		return
	}
	for _, sess := range sessions {
		if sess.WorktreePath == "" {
			continue
		}
		dirty, err := git.IsWorktreeDirty(sess.WorktreePath)
		if err != nil {
			continue
		}
		s.db.UpdateSessionDirty(sess.ID, dirty)
	}
}
