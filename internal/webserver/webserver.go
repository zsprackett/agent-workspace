package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/events"
)

type Config struct {
	Enabled bool
	Port    int
	Host    string
}

type Server struct {
	store   *db.DB
	cfg     Config
	mu      sync.Mutex
	clients map[chan events.Event]struct{}
}

func New(store *db.DB, cfg Config) *Server {
	return &Server{
		store:   store,
		cfg:     cfg,
		clients: make(map[chan events.Event]struct{}),
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
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions/{id}/notes", s.handleUpdateNotes)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSessionEvents)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.Handle("GET /", http.FileServer(staticFiles()))
	return mux
}

func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("webserver: %v\n", err)
		}
	}()
	return nil
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
