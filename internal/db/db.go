package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}
	return &DB{sql: conn}, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) Migrate() error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create metadata: %w", err)
	}

	_, err = d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id                TEXT PRIMARY KEY,
			title             TEXT NOT NULL,
			project_path      TEXT NOT NULL,
			group_path        TEXT NOT NULL DEFAULT 'my-sessions',
			sort_order        INTEGER NOT NULL DEFAULT 0,
			command           TEXT NOT NULL DEFAULT '',
			tool              TEXT NOT NULL DEFAULT 'shell',
			status            TEXT NOT NULL DEFAULT 'idle',
			tmux_session      TEXT NOT NULL DEFAULT '',
			created_at        INTEGER NOT NULL,
			last_accessed     INTEGER NOT NULL DEFAULT 0,
			parent_session_id TEXT NOT NULL DEFAULT '',
			worktree_path     TEXT NOT NULL DEFAULT '',
			worktree_repo     TEXT NOT NULL DEFAULT '',
			worktree_branch   TEXT NOT NULL DEFAULT '',
			acknowledged      INTEGER NOT NULL DEFAULT 0,
			repo_url          TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("create sessions: %w", err)
	}

	// Add repo_url column to existing DBs; ignore "duplicate column" errors.
	if _, alterErr := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN repo_url TEXT NOT NULL DEFAULT ''`); alterErr != nil {
		if !isDuplicateColumnError(alterErr) {
			return fmt.Errorf("alter sessions add repo_url: %w", alterErr)
		}
	}

	// Add has_uncommitted column to existing DBs; ignore "duplicate column" errors.
	if _, alterErr := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN has_uncommitted INTEGER NOT NULL DEFAULT 0`); alterErr != nil {
		if !isDuplicateColumnError(alterErr) {
			return fmt.Errorf("alter sessions add has_uncommitted: %w", alterErr)
		}
	}

	// Add notes column to existing DBs; ignore "duplicate column" errors.
	if _, alterErr := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN notes TEXT NOT NULL DEFAULT ''`); alterErr != nil {
		if !isDuplicateColumnError(alterErr) {
			return fmt.Errorf("alter sessions add notes: %w", alterErr)
		}
	}

	_, err = d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS groups (
			path         TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			expanded     INTEGER NOT NULL DEFAULT 1,
			sort_order   INTEGER NOT NULL DEFAULT 0,
			default_path TEXT NOT NULL DEFAULT '',
			repo_url     TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("create groups: %w", err)
	}

	// Add repo_url column to existing groups tables; ignore "duplicate column" errors.
	if _, alterErr := d.sql.Exec(`ALTER TABLE groups ADD COLUMN repo_url TEXT NOT NULL DEFAULT ''`); alterErr != nil {
		if !isDuplicateColumnError(alterErr) {
			return fmt.Errorf("alter groups add repo_url: %w", alterErr)
		}
	}

	// Add default_tool column to existing groups tables; ignore "duplicate column" errors.
	if _, alterErr := d.sql.Exec(`ALTER TABLE groups ADD COLUMN default_tool TEXT NOT NULL DEFAULT ''`); alterErr != nil {
		if !isDuplicateColumnError(alterErr) {
			return fmt.Errorf("alter groups add default_tool: %w", alterErr)
		}
	}

	_, err = d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS session_events (
			id         INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			ts         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			event_type TEXT NOT NULL,
			detail     TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("create session_events: %w", err)
	}

	if _, alterErr := d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_session_events_session_id ON session_events(session_id, ts DESC)`); alterErr != nil {
		return fmt.Errorf("index session_events: %w", alterErr)
	}

	return nil
}

func (d *DB) SaveSession(s *Session) error {
	_, err := d.sql.Exec(`
		INSERT OR REPLACE INTO sessions (
			id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.ID, s.Title, s.ProjectPath, s.GroupPath, s.SortOrder,
		s.Command, string(s.Tool), string(s.Status), s.TmuxSession,
		s.CreatedAt.UnixMilli(), s.LastAccessed.UnixMilli(),
		s.ParentSessionID, s.WorktreePath, s.WorktreeRepo, s.WorktreeBranch,
		boolToInt(s.Acknowledged), s.RepoURL, boolToInt(s.HasUncommitted), s.Notes,
	)
	return err
}

func (d *DB) GetSession(id string) (*Session, error) {
	row := d.sql.QueryRow(`
		SELECT id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func (d *DB) GetSessionByTmuxName(tmuxSession string) (*Session, error) {
	row := d.sql.QueryRow(`
		SELECT id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		FROM sessions WHERE tmux_session = ?`, tmuxSession)
	return scanSession(row)
}

func (d *DB) LoadSessions() ([]*Session, error) {
	rows, err := d.sql.Query(`
		SELECT id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		FROM sessions ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (d *DB) LoadSessionsByGroupPath(groupPath string) ([]*Session, error) {
	rows, err := d.sql.Query(`
		SELECT id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		FROM sessions WHERE group_path = ? ORDER BY sort_order`, groupPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (d *DB) DeleteSession(id string) error {
	_, err := d.sql.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

func (d *DB) WriteStatus(id string, status SessionStatus, tool Tool) error {
	_, err := d.sql.Exec("UPDATE sessions SET status = ?, tool = ? WHERE id = ?",
		string(status), string(tool), id)
	return err
}

func (d *DB) UpdateSessionField(id, field string, value any) error {
	columnMap := map[string]string{
		"title":           "title",
		"project_path":    "project_path",
		"group_path":      "group_path",
		"sort_order":      "sort_order",
		"tmux_session":    "tmux_session",
		"last_accessed":   "last_accessed",
		"worktree_path":   "worktree_path",
		"worktree_repo":   "worktree_repo",
		"worktree_branch": "worktree_branch",
		"acknowledged":    "acknowledged",
	}
	col, ok := columnMap[field]
	if !ok {
		return fmt.Errorf("unknown field: %s", field)
	}
	_, err := d.sql.Exec(fmt.Sprintf("UPDATE sessions SET %s = ? WHERE id = ?", col), value, id)
	return err
}

func (d *DB) SetAcknowledged(id string, ack bool) error {
	_, err := d.sql.Exec("UPDATE sessions SET acknowledged = ? WHERE id = ?", boolToInt(ack), id)
	return err
}

func (d *DB) UpdateSessionDirty(id string, dirty bool) error {
	_, err := d.sql.Exec("UPDATE sessions SET has_uncommitted = ? WHERE id = ?", boolToInt(dirty), id)
	return err
}

func (d *DB) UpdateSessionNotes(id, notes string) error {
	_, err := d.sql.Exec("UPDATE sessions SET notes = ? WHERE id = ?", notes, id)
	return err
}

// rowScanner is implemented by both *sql.Row and *sql.Rows
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner) (*Session, error) {
	var s Session
	var tool, status string
	var createdAt, lastAccessed int64
	var ack, hasUncommitted int
	err := row.Scan(
		&s.ID, &s.Title, &s.ProjectPath, &s.GroupPath, &s.SortOrder,
		&s.Command, &tool, &status, &s.TmuxSession,
		&createdAt, &lastAccessed,
		&s.ParentSessionID, &s.WorktreePath, &s.WorktreeRepo, &s.WorktreeBranch,
		&ack, &s.RepoURL, &hasUncommitted, &s.Notes,
	)
	if err != nil {
		return nil, err
	}
	s.Tool = Tool(tool)
	s.Status = SessionStatus(status)
	s.CreatedAt = time.UnixMilli(createdAt)
	s.LastAccessed = time.UnixMilli(lastAccessed)
	s.Acknowledged = ack == 1
	s.HasUncommitted = hasUncommitted == 1
	return &s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isDuplicateColumnError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

func (d *DB) SaveGroups(groups []*Group) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM groups"); err != nil {
		return err
	}
	for _, g := range groups {
		if _, err := tx.Exec(
			"INSERT INTO groups (path, name, expanded, sort_order, default_path, repo_url, default_tool) VALUES (?,?,?,?,?,?,?)",
			g.Path, g.Name, boolToInt(g.Expanded), g.SortOrder, g.DefaultPath, g.RepoURL, string(g.DefaultTool),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) LoadGroups() ([]*Group, error) {
	rows, err := d.sql.Query("SELECT path, name, expanded, sort_order, default_path, repo_url, default_tool FROM groups ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []*Group
	for rows.Next() {
		var g Group
		var expanded int
		var defaultTool string
		if err := rows.Scan(&g.Path, &g.Name, &expanded, &g.SortOrder, &g.DefaultPath, &g.RepoURL, &defaultTool); err != nil {
			return nil, err
		}
		g.Expanded = expanded == 1
		g.DefaultTool = Tool(defaultTool)
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

func (d *DB) DeleteGroup(path string) error {
	_, err := d.sql.Exec("DELETE FROM groups WHERE path = ?", path)
	return err
}

func (d *DB) SetMeta(key, value string) error {
	_, err := d.sql.Exec("INSERT OR REPLACE INTO metadata (key, value) VALUES (?,?)", key, value)
	return err
}

func (d *DB) GetMeta(key string) (string, error) {
	var value string
	err := d.sql.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (d *DB) Touch() error {
	return d.SetMeta("last_modified", fmt.Sprintf("%d", time.Now().UnixMilli()))
}

func (d *DB) LastModified() int64 {
	v, _ := d.GetMeta("last_modified")
	if v == "" {
		return 0
	}
	var ts int64
	fmt.Sscanf(v, "%d", &ts)
	return ts
}

func (d *DB) IsEmpty() (bool, error) {
	var count int
	err := d.sql.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	return count == 0, err
}

func (d *DB) InsertSessionEvent(sessionID, eventType, detail string) error {
	_, err := d.sql.Exec(
		`INSERT INTO session_events (session_id, event_type, detail) VALUES (?, ?, ?)`,
		sessionID, eventType, detail,
	)
	return err
}

func (d *DB) GetSessionEvents(sessionID string, limit int) ([]SessionEvent, error) {
	rows, err := d.sql.Query(
		`SELECT id, session_id, ts, event_type, detail
		 FROM session_events
		 WHERE session_id = ?
		 ORDER BY ts DESC, id DESC
		 LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SessionEvent
	for rows.Next() {
		var e SessionEvent
		var ts string
		if err := rows.Scan(&e.ID, &e.SessionID, &ts, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		e.Ts, _ = time.Parse("2006-01-02 15:04:05", ts)
		events = append(events, e)
	}
	return events, rows.Err()
}
