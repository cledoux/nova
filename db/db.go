package db

import (
	"database/sql"
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
	Status     string
	CreatedAt  time.Time
	LastActive time.Time
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
			status       TEXT NOT NULL DEFAULT 'cold',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_active  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
		`INSERT INTO sessions (id, name, claude_sid, workspace, channel_id, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Name, sess.ClaudeSID, sess.Workspace, sess.ChannelID, sess.Status,
	)
	return err
}

// GetSession returns the session with the given ID, or an error if not found.
func (s *Store) GetSession(id string) (Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, name, claude_sid, workspace, channel_id, status, created_at, last_active
		 FROM sessions WHERE id = ?`, id,
	))
}

// GetSessionByName returns the session with the given name.
func (s *Store) GetSessionByName(name string) (Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, name, claude_sid, workspace, channel_id, status, created_at, last_active
		 FROM sessions WHERE name = ?`, name,
	))
}

// GetSessionByChannelID returns the most recently active non-terminated session
// for the given Discord channel ID.
func (s *Store) GetSessionByChannelID(channelID string) (Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, name, claude_sid, workspace, channel_id, status, created_at, last_active
		 FROM sessions WHERE channel_id = ? AND status != 'terminated'
		 ORDER BY last_active DESC LIMIT 1`, channelID,
	))
}

// ListSessions returns all non-terminated sessions.
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, name, claude_sid, workspace, channel_id, status, created_at, last_active
		 FROM sessions WHERE status != 'terminated' ORDER BY created_at`,
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

// ListTerminatedSessions returns sessions with status 'terminated'.
func (s *Store) ListTerminatedSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, name, claude_sid, workspace, channel_id, status, created_at, last_active
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
		&sess.ChannelID, &sess.Status,
		&sess.CreatedAt, &sess.LastActive,
	)
	return sess, err
}
