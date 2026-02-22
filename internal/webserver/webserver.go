package webserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
	"github.com/zsprackett/agent-workspace/internal/git"
	"github.com/zsprackett/agent-workspace/internal/session"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

type TLSConfig struct {
	Mode     string // "self-signed", "autocert", "manual", or "" (plain HTTP)
	Domain   string // autocert only
	CertFile string // manual only
	KeyFile  string // manual only
	CacheDir string // autocert + self-signed
}

type AuthConfig struct {
	JWTSecret       string
	RefreshTokenTTL string // parsed duration, e.g. "168h"
}

type Config struct {
	Enabled           bool
	Port              int
	Host              string
	TLS               TLSConfig
	Auth              AuthConfig
	ReposDir          string
	WorktreesDir      string
	DefaultBaseBranch string
}

type Server struct {
	store   *db.DB
	manager *session.Manager
	cfg     Config
	mu      sync.Mutex
	clients map[chan events.Event]struct{}
	ttyd    *ttydManager
}

func New(store *db.DB, manager *session.Manager, cfg Config) *Server {
	return &Server{
		store:   store,
		manager: manager,
		cfg:     cfg,
		clients: make(map[chan events.Event]struct{}),
		ttyd:    newTTYDManager(),
	}
}

// Broadcast implements events.Broadcaster.
func (s *Server) Broadcast(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Server) addClient(ch chan events.Event) {
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeClient(ch chan events.Event) {
	s.mu.Lock()
	delete(s.clients, ch)
	s.mu.Unlock()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cert", s.handleCert)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /api/sessions/{id}/notes", s.handleUpdateNotes)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopSession)
	mux.HandleFunc("POST /api/sessions/{id}/restart", s.handleRestartSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSessionEvents)
	mux.HandleFunc("DELETE /api/sessions/{id}/ttyd", s.handleKillTTYD)
	mux.HandleFunc("GET /api/usage", s.handleUsage)
	mux.HandleFunc("GET /api/sessions/{id}/git/status", s.handleGitStatus)
	mux.HandleFunc("GET /api/sessions/{id}/git/diff", s.handleGitDiff)
	mux.HandleFunc("GET /api/sessions/{id}/git/status/text", s.handleGitStatusText)
	mux.HandleFunc("GET /api/sessions/{id}/git/diff/text", s.handleGitDiffText)
	mux.HandleFunc("GET /api/sessions/{id}/pr-url", s.handlePRURL)
	mux.HandleFunc("GET /terminal/{id}/", s.handleTerminalProxy)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFS, "static/login.html")
	})
	mux.Handle("GET /", http.FileServer(staticFiles()))

	if s.cfg.Auth.JWTSecret == "" {
		return mux
	}
	has, _ := s.store.HasAnyAccount()
	if !has {
		return mux
	}
	return jwtMiddleware(s.cfg.Auth.JWTSecret, mux)
}


func (s *Server) tlsCacheDir() string {
	if s.cfg.TLS.CacheDir != "" {
		return s.cfg.TLS.CacheDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-workspace", "certs")
}

func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}

	handler := s.Handler()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	srv := &http.Server{Addr: addr, Handler: handler}

	switch s.cfg.TLS.Mode {
	case "autocert":
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(s.cfg.TLS.Domain),
			Cache:      autocert.DirCache(s.tlsCacheDir()),
		}
		srv.TLSConfig = m.TLSConfig()
		// Serve HTTP-01 ACME challenges and redirect plain HTTP to HTTPS.
		go http.ListenAndServe(":80", m.HTTPHandler(nil))
		go func() {
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				fmt.Printf("webserver: %v\n", err)
			}
		}()

	case "self-signed":
		tlsCfg, err := selfSignedTLS(s.tlsCacheDir())
		if err != nil {
			return fmt.Errorf("self-signed TLS: %w", err)
		}
		srv.TLSConfig = tlsCfg
		go func() {
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				fmt.Printf("webserver: %v\n", err)
			}
		}()

	case "manual":
		go func() {
			if err := srv.ListenAndServeTLS(s.cfg.TLS.CertFile, s.cfg.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				fmt.Printf("webserver: %v\n", err)
			}
		}()

	default: // plain HTTP
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("webserver: %v\n", err)
			}
		}()
	}

	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	acc, err := s.store.GetAccountByUsername(body.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(body.Password)) != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	ttl, err := time.ParseDuration(s.cfg.Auth.RefreshTokenTTL)
	if err != nil || ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	accessToken, err := IssueAccessToken(s.cfg.Auth.JWTSecret, acc.Username, time.Hour)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	if err := s.store.CreateRefreshToken(refreshToken, acc.ID, time.Now().Add(ttl)); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	rt, err := s.store.GetRefreshToken(body.RefreshToken)
	if err != nil || time.Now().After(rt.ExpiresAt) {
		http.Error(w, "unauthorized", 401)
		return
	}
	acc, err := s.store.GetAccountByID(rt.AccountID)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	// Rotate: delete old, issue new
	s.store.DeleteRefreshToken(body.RefreshToken)

	ttl, err := time.ParseDuration(s.cfg.Auth.RefreshTokenTTL)
	if err != nil || ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	accessToken, _ := IssueAccessToken(s.cfg.Auth.JWTSecret, acc.Username, time.Hour)
	newRefreshToken, _ := GenerateRefreshToken()
	s.store.CreateRefreshToken(newRefreshToken, acc.ID, time.Now().Add(ttl))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.store.DeleteRefreshToken(body.RefreshToken)
	w.WriteHeader(204)
}

type sessionsResponse struct {
	Sessions []*db.Session `json:"sessions"`
	Groups   []*db.Group   `json:"groups"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.LoadSessions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	groups, err := s.store.LoadGroups()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionsResponse{Sessions: sessions, Groups: groups})
}

func (s *Server) handleUpdateNotes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.store.UpdateSessionNotes(id, body.Notes); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "notes_updated", SessionID: id})
	w.WriteHeader(204)
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	evts, err := s.store.GetSessionEvents(id, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"events": evts})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	ch := make(chan events.Event, 16)
	s.addClient(ch)
	defer s.removeClient(ch)

	writeSSE(w, flusher, events.Event{Type: "snapshot"})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(w, flusher, e)
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, f http.Flusher, e events.Event) {
	data, _ := json.Marshal(e)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string  `json:"title"`
		Tool        db.Tool `json:"tool"`
		GroupPath   string  `json:"group_path"`
		ProjectPath string  `json:"project_path"`
		Command     string  `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Look up the group to check for a repo URL (worktree flow).
	var groupRepoURL, preLaunchCmd string
	if body.GroupPath != "" {
		groups, _ := s.store.LoadGroups()
		for _, g := range groups {
			if g.Path == body.GroupPath {
				groupRepoURL = g.RepoURL
				preLaunchCmd = g.PreLaunchCommand
				break
			}
		}
	}

	if groupRepoURL != "" {
		s.handleCreateWorktreeSession(w, body.Title, body.Tool, body.GroupPath, body.Command, groupRepoURL, preLaunchCmd)
		return
	}

	if body.ProjectPath == "" {
		http.Error(w, "project path is required for groups without a repo URL", 400)
		return
	}

	sess, err := s.manager.Create(session.CreateOptions{
		Title:       body.Title,
		Tool:        body.Tool,
		GroupPath:   body.GroupPath,
		ProjectPath: body.ProjectPath,
		Command:     body.Command,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "refresh"})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(sess)
}

func (s *Server) handleCreateWorktreeSession(w http.ResponseWriter, title string, tool db.Tool, groupPath, command, repoURL, preLaunchCmd string) {
	host, owner, repo, err := git.ParseRepoURL(repoURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid repo URL: %v", err), 400)
		return
	}

	if title == "" {
		title = session.GenerateTitle()
	}
	if command == "" {
		command = db.ToolCommand(tool, "")
	}

	// Insert a pending row immediately.
	sessions, _ := s.store.LoadSessions()
	now := time.Now()
	sessionID := uuid.NewString()
	pending := &db.Session{
		ID:           sessionID,
		Title:        title,
		GroupPath:    groupPath,
		Tool:         tool,
		Command:      command,
		Status:       db.StatusCreating,
		CreatedAt:    now,
		LastAccessed: now,
		SortOrder:    len(sessions),
		RepoURL:      repoURL,
	}
	if err := s.store.SaveSession(pending); err != nil {
		http.Error(w, fmt.Sprintf("create failed: %v", err), 500)
		return
	}
	_ = s.store.InsertSessionEvent(sessionID, "created", "")
	s.store.Touch()
	s.Broadcast(events.Event{Type: "refresh"})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(pending)

	bareRepoPath := git.BareRepoPath(s.cfg.ReposDir, host, owner, repo)
	branch := git.SanitizeBranchName(title)
	wtPath := git.WorktreePath(s.cfg.WorktreesDir, host, owner, repo, branch)

	cancelCreate := func(msg string) {
		s.store.DeleteSession(sessionID)
		_ = s.store.Touch()
		s.Broadcast(events.Event{Type: "refresh"})
	}

	go func() {
		// 1. Ensure bare repo directory exists.
		if err := os.MkdirAll(filepath.Dir(bareRepoPath), 0755); err != nil {
			cancelCreate(fmt.Sprintf("create repos dir failed: %v", err))
			return
		}

		// 2. Clone or fetch the bare repo.
		if !git.IsBareRepo(bareRepoPath) {
			if err := git.CloneBare(repoURL, bareRepoPath); err != nil {
				cancelCreate(fmt.Sprintf("clone failed: %v", err))
				return
			}
		} else {
			if err := git.FetchBare(bareRepoPath); err != nil {
				cancelCreate(fmt.Sprintf("fetch failed: %v", err))
				return
			}
		}

		// 3. Ensure worktree parent directory exists.
		if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
			cancelCreate(fmt.Sprintf("create worktrees dir failed: %v", err))
			return
		}

		// 4. Create the worktree; reuse if it already exists.
		baseBranch := s.cfg.DefaultBaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		if _, err := git.CreateWorktree(bareRepoPath, branch, wtPath, baseBranch); err != nil && !errors.Is(err, git.ErrWorktreeExists) {
			cancelCreate(fmt.Sprintf("create worktree failed: %v", err))
			return
		}

		// 5. Run pre-launch command if set.
		if preLaunchCmd != "" {
			toolCmd := db.ToolCommand(tool, "")
			out, err := session.RunPreLaunchCommand(preLaunchCmd, toolCmd, bareRepoPath, wtPath)
			if err != nil {
				cancelCreate(fmt.Sprintf("pre-launch command failed: %v\n%s", err, out))
				return
			}
		}

		// 6. Create the tmux session.
		tmuxName := tmux.GenerateSessionName(title)
		if err := tmux.CreateSession(tmux.CreateOptions{
			Name:    tmuxName,
			Command: command,
			Cwd:     wtPath,
		}); err != nil {
			cancelCreate(fmt.Sprintf("create tmux session failed: %v", err))
			return
		}

		// 7. Update the DB row to running.
		pending.TmuxSession = tmuxName
		pending.Status = db.StatusRunning
		pending.ProjectPath = wtPath
		pending.WorktreePath = wtPath
		pending.WorktreeRepo = bareRepoPath
		pending.WorktreeBranch = branch
		pending.LastAccessed = time.Now()
		if err := s.store.SaveSession(pending); err != nil {
			tmux.KillSession(tmuxName)
			cancelCreate(fmt.Sprintf("save failed: %v", err))
			return
		}
		_ = s.store.Touch()
		s.Broadcast(events.Event{Type: "refresh"})
	}()
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Stop(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "refresh"})
	w.WriteHeader(204)
}

func (s *Server) handleRestartSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Restart(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "refresh"})
	w.WriteHeader(204)
}

// handleTerminalProxy spawns ttyd on demand and reverse-proxies all traffic
// (HTTP + WebSocket) through our server so remote clients (e.g. iOS) can reach
// it without needing direct access to 127.0.0.1:<port>.
func (s *Server) handleTerminalProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(id)
	if err != nil || sess == nil || sess.TmuxSession == "" {
		http.Error(w, "session not found", 404)
		return
	}
	port, err := s.ttyd.spawn(id, sess.TmuxSession)
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
}

// handleCert serves the self-signed certificate as a download so clients
// (e.g. iOS) can install and trust it. Intentionally unauthenticated.
func (s *Server) handleCert(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TLS.Mode != "self-signed" {
		http.Error(w, "not using self-signed TLS", http.StatusNotFound)
		return
	}
	certFile := filepath.Join(s.tlsCacheDir(), "self-signed.crt")
	data, err := os.ReadFile(certFile)
	if err != nil {
		http.Error(w, "cert not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="agent-workspace.crt"`)
	w.Write(data)
}

func (s *Server) handleKillTTYD(w http.ResponseWriter, r *http.Request) {
	s.ttyd.kill(r.PathValue("id"))
	w.WriteHeader(204)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	latest, _ := s.store.GetLatestUsageSnapshot()
	history, _ := s.store.GetUsageSnapshots(48)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"latest":  latest,
		"history": history,
	})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Delete(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Broadcast(events.Event{Type: "refresh"})
	w.WriteHeader(204)
}
