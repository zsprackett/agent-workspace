package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsClientMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(id)
	if err != nil || sess == nil || sess.TmuxSession == "" {
		http.Error(w, "session not found", 404)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// First message must be {type:"init", cols:N, rows:N} sent by the client
	// immediately after the WebSocket opens. Resize the pane to match the web
	// terminal width so subsequent output is rendered at the correct column count.
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var initMsg wsClientMsg
	if json.Unmarshal(raw, &initMsg) == nil && initMsg.Type == "init" &&
		initMsg.Cols > 0 && initMsg.Rows > 0 {
		tmux.ResizePane(sess.TmuxSession, initMsg.Cols, initMsg.Rows)
	}

	// No initial capture: CapturePane returns pre-rendered rows at the old
	// terminal width which wrap incorrectly in xterm. Instead, just start
	// pipe-pane; the first full repaint the application emits (htop ~1s,
	// Claude Code on next output) will be at the correct post-resize width.

	// Start pipe-pane streaming to a temp file.
	pipeFile := fmt.Sprintf("/tmp/agws-term-%s", id)
	os.Remove(pipeFile)
	tmux.PipePane(sess.TmuxSession, "cat >> "+pipeFile)

	// Open pipe file for tail-follow reading.
	f, err := os.OpenFile(pipeFile, os.O_RDONLY|os.O_CREATE, 0600)
	if err == nil {
		f.Seek(0, io.SeekEnd)
		go func() {
			defer f.Close()
			buf := make([]byte, 4096)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				n, readErr := f.Read(buf)
				if n > 0 {
					if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
						cancel()
						return
					}
				}
				if readErr == io.EOF {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				if readErr != nil {
					cancel()
					return
				}
			}
		}()
	}

	defer func() {
		tmux.StopPipePane(sess.TmuxSession)
		os.Remove(pipeFile)
	}()

	// Read loop: handle input and resize messages from the client.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg wsClientMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "input":
			tmux.SendText(sess.TmuxSession, msg.Data)
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				tmux.ResizePane(sess.TmuxSession, msg.Cols, msg.Rows)
			}
		}
	}
}
