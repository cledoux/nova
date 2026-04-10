package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Session represents a persisted Claude session record.
type Session struct {
	ID         string
	Name       string
	ClaudeSID  string
	Workspace  string
	ChannelID  string
	SwarmID    string
	Status     string
	CreatedAt  time.Time
	LastActive time.Time
}

// Swarm represents a persisted swarm record.
type Swarm struct {
	ID         string
	Name       string
	CategoryID string
	OrchID     string
	CreatedAt  time.Time
}

// Store wraps a SQLite database and provides all query methods.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL UNIQUE,
			claude_sid   TEXT NOT NULL DEFAULT '',
			workspace    TEXT NOT NULL,
			channel_id   TEXT NOT NULL,
			swarm_id     TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'cold',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_active  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS swarms (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL UNIQUE,
			category_id  TEXT NOT NULL,
			orch_id      TEXT NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS messages (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id   TEXT NOT NULL,
			role         TEXT NOT NULL,
			content      TEXT NOT NULL,
			ts           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

// CreateSession inserts a new session record.
func (s *Store) CreateSession(sess Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, name, claude_sid, workspace, channel_id, swarm_id, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Name, sess.ClaudeSID, sess.Workspace, sess.ChannelID, sess.SwarmID, sess.Status,
	)
	return err
}

// GetSession returns the session with the given ID, or an error if not found.
func (s *Store) GetSession(id string) (Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, name, claude_sid, workspace, channel_id, swarm_id, status, created_at, last_active
		 FROM sessions WHERE id = ?`, id,
	))
}

// GetSessionByName returns the session with the given name.
func (s *Store) GetSessionByName(name string) (Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, name, claude_sid, workspace, channel_id, swarm_id, status, created_at, last_active
		 FROM sessions WHERE name = ?`, name,
	))
}

// ListSessions returns all sessions. If swarmID is non-empty, filters to that swarm.
func (s *Store) ListSessions(swarmID string) ([]Session, error) {
	var rows *sql.Rows
	var err error
	if swarmID == "" {
		rows, err = s.db.Query(
			`SELECT id, name, claude_sid, workspace, channel_id, swarm_id, status, created_at, last_active
			 FROM sessions WHERE status != 'terminated' ORDER BY created_at`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, name, claude_sid, workspace, channel_id, swarm_id, status, created_at, last_active
			 FROM sessions WHERE swarm_id = ? AND status != 'terminated' ORDER BY created_at`, swarmID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ListTerminatedSessions returns sessions with status 'terminated'.
func (s *Store) ListTerminatedSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, name, claude_sid, workspace, channel_id, swarm_id, status, created_at, last_active
		 FROM sessions WHERE status = 'terminated'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// UpdateSessionStatus sets the status field for the given session ID.
func (s *Store) UpdateSessionStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE sessions SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateSessionClaudeSID stores the claude CLI session ID for a session.
func (s *Store) UpdateSessionClaudeSID(id, claudeSID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET claude_sid = ? WHERE id = ?`, claudeSID, id)
	return err
}

// TouchSession updates last_active to now.
func (s *Store) TouchSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_active = strftime('%Y-%m-%d %H:%M:%f', 'now') WHERE id = ?`, id)
	return err
}

// ResetActiveSessions sets all 'hot' sessions to 'cold' (called on startup).
func (s *Store) ResetActiveSessions() error {
	_, err := s.db.Exec(`UPDATE sessions SET status = 'cold' WHERE status = 'hot'`)
	return err
}

// CreateSwarm inserts a new swarm record.
func (s *Store) CreateSwarm(sw Swarm) error {
	_, err := s.db.Exec(
		`INSERT INTO swarms (id, name, category_id, orch_id) VALUES (?, ?, ?, ?)`,
		sw.ID, sw.Name, sw.CategoryID, sw.OrchID,
	)
	return err
}

// GetSwarm returns the swarm with the given ID.
func (s *Store) GetSwarm(id string) (Swarm, error) {
	return s.scanSwarm(s.db.QueryRow(
		`SELECT id, name, category_id, orch_id, created_at FROM swarms WHERE id = ?`, id,
	))
}

// GetSwarmByName returns the swarm with the given name.
func (s *Store) GetSwarmByName(name string) (Swarm, error) {
	return s.scanSwarm(s.db.QueryRow(
		`SELECT id, name, category_id, orch_id, created_at FROM swarms WHERE name = ?`, name,
	))
}

// ListSwarms returns all swarms.
func (s *Store) ListSwarms() ([]Swarm, error) {
	rows, err := s.db.Query(`SELECT id, name, category_id, orch_id, created_at FROM swarms ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Swarm
	for rows.Next() {
		sw, err := s.scanSwarm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sw)
	}
	return out, rows.Err()
}

// DeleteSwarm removes the swarm record with the given ID.
func (s *Store) DeleteSwarm(id string) error {
	res, err := s.db.Exec(`DELETE FROM swarms WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("swarm not found")
	}
	return nil
}

// InsertMessage records a message for a session.
func (s *Store) InsertMessage(sessionID, role, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)`,
		sessionID, role, content,
	)
	return err
}

// CountMessages returns the number of messages for a session.
func (s *Store) CountMessages(sessionID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&n)
	return n, err
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanSession(row scanner) (Session, error) {
	var sess Session
	err := row.Scan(
		&sess.ID, &sess.Name, &sess.ClaudeSID, &sess.Workspace,
		&sess.ChannelID, &sess.SwarmID, &sess.Status,
		&sess.CreatedAt, &sess.LastActive,
	)
	return sess, err
}

func (s *Store) scanSwarm(row scanner) (Swarm, error) {
	var sw Swarm
	err := row.Scan(&sw.ID, &sw.Name, &sw.CategoryID, &sw.OrchID, &sw.CreatedAt)
	return sw, err
}
