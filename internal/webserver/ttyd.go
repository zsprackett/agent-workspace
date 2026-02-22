package webserver

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"
)

type ttydProc struct {
	cmd  *exec.Cmd
	port int
}

type ttydManager struct {
	mu    sync.Mutex
	procs map[string]*ttydProc
}

func newTTYDManager() *ttydManager {
	return &ttydManager{procs: make(map[string]*ttydProc)}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// waitReady polls the port until it accepts a connection or the timeout elapses.
func waitReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("ttyd did not start on port %d within %s", port, timeout)
}

// spawn starts a ttyd instance for the given session if one isn't already running.
// Returns the port it is listening on.
func (m *ttydManager) spawn(sessionID, tmuxSession string) (int, error) {
	m.mu.Lock()
	if p, ok := m.procs[sessionID]; ok && p.cmd.ProcessState == nil {
		port := p.port
		m.mu.Unlock()
		return port, nil
	}
	delete(m.procs, sessionID)
	m.mu.Unlock()

	port, err := freePort()
	if err != nil {
		return 0, err
	}

	// Enable mouse mode so scroll works in the web terminal.
	exec.Command("tmux", "set-option", "-t", tmuxSession, "mouse", "on").Run()

	cmd := exec.Command("ttyd",
		"--port", fmt.Sprintf("%d", port),
		"--once",
		"--writable",
		"--base-path", "/terminal/"+sessionID,
		"tmux", "attach-session", "-t", tmuxSession,
	)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start ttyd: %w", err)
	}

	m.mu.Lock()
	m.procs[sessionID] = &ttydProc{cmd: cmd, port: port}
	m.mu.Unlock()

	go func() {
		cmd.Wait()
		m.mu.Lock()
		if p, ok := m.procs[sessionID]; ok && p.cmd == cmd {
			delete(m.procs, sessionID)
		}
		m.mu.Unlock()
	}()

	if err := waitReady(port, 3*time.Second); err != nil {
		m.kill(sessionID)
		return 0, err
	}

	return port, nil
}

func (m *ttydManager) kill(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[sessionID]; ok {
		p.cmd.Process.Kill()
		delete(m.procs, sessionID)
	}
}
