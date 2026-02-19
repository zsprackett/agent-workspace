package syncer

import (
	"log"
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
}

func New(store *db.DB, reposDir string) *Syncer {
	return &Syncer{
		db:       store,
		reposDir: reposDir,
		interval: 2 * time.Minute,
		stop:     make(chan struct{}),
		fetch:    git.FetchBare,
	}
}

// NewWithFetch creates a Syncer with an injectable fetch function. Used in tests.
func NewWithFetch(store *db.DB, reposDir string, fetch func(repoDir string) error) *Syncer {
	s := New(store, reposDir)
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
			// Non-fatal: log to stderr and continue
			log.Printf("syncer: fetch %s: %v", path, err)
		}
	}
}
