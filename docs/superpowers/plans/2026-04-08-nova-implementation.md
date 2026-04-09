# Nova Swarm Controller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone Discord bot that spawns and orchestrates Claude Code CLI instances, with Discord channels as the communication medium.

**Architecture:** Hybrid hot/cold session model — active sessions are live subprocesses with piped stdin/stdout; idle sessions are suspended and resume on next message via the Claude CLI's `--resume` flag. A JSON directive protocol lets Claude agents control the swarm (spawn workers, send messages, create channels).

**Tech Stack:** Go, discordgo v0.29.0, modernc.org/sqlite, BurntSushi/toml, google/uuid

---

### Task 0: CLI Spike — Verify claude flags and session ID capture

**Goal:** Determine the exact `claude` CLI flags for resuming sessions and injecting system prompts, and discover how to capture a session's ID after first spawn.

**Files:**
- Create: `docs/nova-cli-spike.md`

**Acceptance Criteria:**
- [ ] Exact flag for resuming a prior session is documented
- [ ] Exact flag for system prompt injection is documented (or confirmed absent)
- [ ] Session ID capture mechanism is documented
- [ ] Fallback approach documented if resume/system-prompt flags don't exist

**Steps:**

- [ ] **Step 1: Explore available flags**

```bash
claude --help 2>&1 | head -80
claude --help 2>&1 | grep -i -E "resume|session|prompt|print|output"
```

- [ ] **Step 2: Check ~/.claude directory structure**

```bash
ls ~/.claude/
ls ~/.claude/projects/ 2>/dev/null || echo "no projects dir"
# Run a quick test session then inspect what was created:
echo "say hello" | claude --print 2>/dev/null
ls -lt ~/.claude/projects/ 2>/dev/null | head -10
```

- [ ] **Step 3: Identify session ID**

After running a test session, inspect the newly-created directory or file to determine the session ID format and how to capture it programmatically (directory diff before/after spawn, or from stdout, or from a flag).

- [ ] **Step 4: Test system prompt injection**

```bash
# Try --system-prompt-file if it exists:
echo "test" > /tmp/sysprompt.txt
echo "what are your instructions?" | claude --system-prompt-file /tmp/sysprompt.txt --print 2>&1
# If that fails, try passing system prompt as first stdin message
```

- [ ] **Step 5: Write findings**

Write `docs/nova-cli-spike.md` documenting: exact flags, session ID location/format, and the capture strategy to use in Task 5.

- [ ] **Step 6: Commit**

```bash
git add docs/nova-cli-spike.md
git commit -m "docs: add CLI spike findings for claude subprocess management"
```

---

### Task 1: Project Scaffold

**Goal:** Working Go module with config loading, minimal main.go, justfile, and service file.

**Files:**
- Create: `go.mod`
- Create: `config/config.go`
- Create: `config/config_test.go`
- Create: `main.go`
- Create: `justfile`
- Create: `nova.service`
- Create: `.gitignore`

**Acceptance Criteria:**
- [ ] `go test ./config/...` passes
- [ ] `go build .` produces a `nova` binary
- [ ] Config loads TOML with correct defaults for missing fields
- [ ] Bot refuses to start with empty `bot_token`

**Verify:** `go test ./config/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write config test**

Create `config/config_test.go`:

```go
package config_test

import (
	"os"
	"testing"

	"nova/config"
)

func TestLoad_defaults(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`bot_token = "tok"`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BotToken != "tok" {
		t.Errorf("BotToken = %q, want %q", cfg.BotToken, "tok")
	}
	if cfg.ControlChannelName != "nova" {
		t.Errorf("ControlChannelName = %q, want nova", cfg.ControlChannelName)
	}
	if cfg.IdleTimeoutMinutes != 10 {
		t.Errorf("IdleTimeoutMinutes = %d, want 10", cfg.IdleTimeoutMinutes)
	}
	if cfg.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q, want claude", cfg.ClaudeBin)
	}
}

func TestLoad_overrides(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("bot_token = \"x\"\nidle_timeout_minutes = 5\ndebug = true\n")
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IdleTimeoutMinutes != 5 {
		t.Errorf("IdleTimeoutMinutes = %d, want 5", cfg.IdleTimeoutMinutes)
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
}

func TestLoad_missingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/nova-config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run test — expect failure**

```bash
go test ./config/... -v
```
Expected: compile error (package doesn't exist yet).

- [ ] **Step 3: Write go.mod**

```
module nova

go 1.22

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/bwmarrin/discordgo v0.29.0
	github.com/google/uuid v1.6.0
	modernc.org/sqlite v1.48.1
)
```

Run `go mod tidy` to generate go.sum.

- [ ] **Step 4: Write config/config.go**

```go
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all Nova bot configuration loaded from config.toml.
type Config struct {
	BotToken           string `toml:"bot_token"`
	GuildID            string `toml:"guild_id"`
	ControlChannelName string `toml:"control_channel_name"`
	SessionRoot        string `toml:"session_root"`
	IdleTimeoutMinutes int    `toml:"idle_timeout_minutes"`
	ClaudeBin          string `toml:"claude_bin"`
	Debug              bool   `toml:"debug"`
}

// Load reads the TOML config file at path. Fields absent from the file keep
// their default values.
func Load(path string) (*Config, error) {
	home, _ := os.UserHomeDir()
	cfg := &Config{
		ControlChannelName: "nova",
		SessionRoot:        filepath.Join(home, ".nova", "sessions"),
		IdleTimeoutMinutes: 10,
		ClaudeBin:          "claude",
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	// Expand leading ~ in SessionRoot.
	if strings.HasPrefix(cfg.SessionRoot, "~/") {
		cfg.SessionRoot = filepath.Join(home, cfg.SessionRoot[2:])
	}
	return cfg, nil
}

// DefaultPath returns ~/.nova/config.toml.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nova", "config.toml")
}
```

- [ ] **Step 5: Run test — expect pass**

```bash
go test ./config/... -v
```
Expected: `PASS`

- [ ] **Step 6: Write main.go**

```go
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"nova/config"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.BotToken == "" {
		log.Fatal("bot_token is required in config.toml")
	}

	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("create discord session: %v", err)
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := dg.Open(); err != nil {
		log.Fatalf("open discord: %v", err)
	}
	defer dg.Close()

	log.Println("Nova running. Ctrl-C to stop.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
```

- [ ] **Step 7: Write justfile**

```makefile
build:
    go build -o nova .

test:
    go test ./...

test-all: build test

fmt:
    gofmt -s -w .
    goimports -w . 2>/dev/null || true

restart:
    systemctl --user restart nova

logs:
    journalctl --user -u nova -f
```

- [ ] **Step 8: Write nova.service**

```ini
[Unit]
Description=Nova Claude Swarm Controller
After=network.target

[Service]
Type=simple
ExecStart=%h/workspace/discord/nova/nova --config %h/.nova/config.toml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
```

- [ ] **Step 9: Write .gitignore**

```
nova
data/
*.db
.env
```

- [ ] **Step 10: Build and commit**

```bash
go build .
git add go.mod go.sum config/ main.go justfile nova.service .gitignore
git commit -m "feat: project scaffold with config loading"
```

---

### Task 2: SQLite Store

**Goal:** Database layer with schema migrations and full CRUD for sessions, swarms, and messages.

**Files:**
- Create: `nova/db/db.go`
- Create: `nova/db/db_test.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/db/... -v` passes
- [ ] Sessions can be created, fetched by ID, fetched by name, listed, status-updated
- [ ] Swarms can be created, fetched by name, listed, deleted
- [ ] `ResetActiveSessions` sets all hot sessions to cold
- [ ] Messages can be inserted and counted

**Verify:** `go test ./nova/db/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write db_test.go**

Create `nova/db/db_test.go`:

```go
package db_test

import (
	"testing"
	"time"

	"nova/nova/db"
)

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetSession(t *testing.T) {
	store := newTestStore(t)
	s := db.Session{
		ID:        "id-1",
		Name:      "worker-1",
		Workspace: "/tmp/ws",
		ChannelID: "ch-1",
		Status:    "cold",
	}
	if err := store.CreateSession(s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := store.GetSession("id-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Name != "worker-1" {
		t.Errorf("Name = %q, want worker-1", got.Name)
	}
}

func TestGetSessionByName(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "id-2", Name: "alpha", Workspace: "/w", ChannelID: "c", Status: "cold"})
	got, err := store.GetSessionByName("alpha")
	if err != nil {
		t.Fatalf("GetSessionByName: %v", err)
	}
	if got.ID != "id-2" {
		t.Errorf("ID = %q, want id-2", got.ID)
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "id-3", Name: "beta", Workspace: "/w", ChannelID: "c", Status: "cold"})
	if err := store.UpdateSessionStatus("id-3", "hot"); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, _ := store.GetSession("id-3")
	if got.Status != "hot" {
		t.Errorf("Status = %q, want hot", got.Status)
	}
}

func TestResetActiveSessions(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "a", Name: "a", Workspace: "/w", ChannelID: "c", Status: "hot"})
	_ = store.CreateSession(db.Session{ID: "b", Name: "b", Workspace: "/w", ChannelID: "c2", Status: "cold"})
	if err := store.ResetActiveSessions(); err != nil {
		t.Fatalf("ResetActiveSessions: %v", err)
	}
	got, _ := store.GetSession("a")
	if got.Status != "cold" {
		t.Errorf("hot session not reset: status = %q", got.Status)
	}
	got2, _ := store.GetSession("b")
	if got2.Status != "cold" {
		t.Errorf("cold session changed: status = %q", got2.Status)
	}
}

func TestListSessions(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "x", Name: "x", Workspace: "/w", ChannelID: "c1", Status: "cold"})
	_ = store.CreateSession(db.Session{ID: "y", Name: "y", Workspace: "/w", ChannelID: "c2", Status: "cold", SwarmID: "sw1"})
	all, err := store.ListSessions("")
	if err != nil || len(all) != 2 {
		t.Errorf("ListSessions all: got %d, want 2 (err=%v)", len(all), err)
	}
	bySwarm, err := store.ListSessions("sw1")
	if err != nil || len(bySwarm) != 1 {
		t.Errorf("ListSessions by swarm: got %d, want 1 (err=%v)", len(bySwarm), err)
	}
}

func TestSwarmCRUD(t *testing.T) {
	store := newTestStore(t)
	sw := db.Swarm{ID: "sw-1", Name: "backend", CategoryID: "cat-1"}
	if err := store.CreateSwarm(sw); err != nil {
		t.Fatalf("CreateSwarm: %v", err)
	}
	got, err := store.GetSwarmByName("backend")
	if err != nil {
		t.Fatalf("GetSwarmByName: %v", err)
	}
	if got.CategoryID != "cat-1" {
		t.Errorf("CategoryID = %q, want cat-1", got.CategoryID)
	}
	if err := store.DeleteSwarm("sw-1"); err != nil {
		t.Fatalf("DeleteSwarm: %v", err)
	}
	_, err = store.GetSwarm("sw-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestInsertMessage(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "s1", Name: "s", Workspace: "/w", ChannelID: "c", Status: "cold"})
	if err := store.InsertMessage("s1", "user", "hello"); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	n, err := store.CountMessages("s1")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if n != 1 {
		t.Errorf("CountMessages = %d, want 1", n)
	}
}

func TestTouchSession(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "t1", Name: "t", Workspace: "/w", ChannelID: "c", Status: "cold"})
	before, _ := store.GetSession("t1")
	time.Sleep(time.Millisecond)
	if err := store.TouchSession("t1"); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	after, _ := store.GetSession("t1")
	if !after.LastActive.After(before.LastActive) {
		t.Error("last_active not updated")
	}
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
go test ./nova/db/... -v
```
Expected: compile error (package not yet written).

- [ ] **Step 3: Write nova/db/db.go**

```go
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
	_, err := s.db.Exec(`UPDATE sessions SET last_active = CURRENT_TIMESTAMP WHERE id = ?`, id)
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
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./nova/db/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add nova/db/
git commit -m "feat: SQLite store with sessions, swarms, messages"
```

---

### Task 3: Directive Parser

**Goal:** Package that parses single-line JSON directives from Claude's stdout.

**Files:**
- Create: `nova/directive/directive.go`
- Create: `nova/directive/directive_test.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/directive/... -v` passes
- [ ] Valid directives are parsed into typed structs
- [ ] Non-JSON lines return `nil, nil`
- [ ] JSON without `"type"` field returns `nil, nil`
- [ ] Malformed JSON returns a non-nil error

**Verify:** `go test ./nova/directive/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write directive_test.go**

Create `nova/directive/directive_test.go`:

```go
package directive_test

import (
	"testing"

	"nova/nova/directive"
)

func TestParse_spawn(t *testing.T) {
	d, err := directive.Parse(`{"type":"spawn","name":"worker-1","task":"implement auth"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d == nil {
		t.Fatal("expected directive, got nil")
	}
	if d.Type != directive.TypeSpawn {
		t.Errorf("Type = %q, want %q", d.Type, directive.TypeSpawn)
	}
	if d.Name != "worker-1" {
		t.Errorf("Name = %q, want worker-1", d.Name)
	}
	if d.Task != "implement auth" {
		t.Errorf("Task = %q, want %q", d.Task, "implement auth")
	}
}

func TestParse_send(t *testing.T) {
	d, err := directive.Parse(`{"type":"send","to":"worker-1","message":"schema ready"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Type != directive.TypeSend {
		t.Errorf("Type = %q, want send", d.Type)
	}
	if d.To != "worker-1" {
		t.Errorf("To = %q, want worker-1", d.To)
	}
	if d.Message != "schema ready" {
		t.Errorf("Message = %q, want schema ready", d.Message)
	}
}

func TestParse_done(t *testing.T) {
	d, err := directive.Parse(`{"type":"done"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d == nil {
		t.Fatal("expected directive")
	}
	if d.Type != directive.TypeDone {
		t.Errorf("Type = %q, want done", d.Type)
	}
}

func TestParse_createChannel(t *testing.T) {
	d, err := directive.Parse(`{"type":"create_channel","name":"design-notes"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Type != directive.TypeCreateChannel {
		t.Errorf("Type = %q, want create_channel", d.Type)
	}
	if d.Name != "design-notes" {
		t.Errorf("Name = %q, want design-notes", d.Name)
	}
}

func TestParse_nonJSON(t *testing.T) {
	d, err := directive.Parse("Hello from Claude!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil for non-JSON, got %+v", d)
	}
}

func TestParse_emptyLine(t *testing.T) {
	d, err := directive.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Errorf("expected nil for empty line, got %+v", d)
	}
}

func TestParse_jsonWithoutType(t *testing.T) {
	d, err := directive.Parse(`{"foo":"bar"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Errorf("expected nil for JSON without type, got %+v", d)
	}
}

func TestParse_malformedJSON(t *testing.T) {
	_, err := directive.Parse(`{"type":"spawn"`)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParse_whitespace(t *testing.T) {
	d, err := directive.Parse(`  {"type":"done"}  `)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d == nil || d.Type != directive.TypeDone {
		t.Errorf("expected done directive, got %+v", d)
	}
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
go test ./nova/directive/... -v
```

- [ ] **Step 3: Write nova/directive/directive.go**

```go
// Package directive parses single-line JSON directives emitted by Claude agents.
package directive

import (
	"encoding/json"
	"strings"
)

// Type identifies the kind of directive.
type Type string

const (
	TypeSpawn         Type = "spawn"
	TypeSend          Type = "send"
	TypeCreateChannel Type = "create_channel"
	TypeDone          Type = "done"
)

// Directive is a parsed swarm control instruction from a Claude agent.
type Directive struct {
	Type    Type   `json:"type"`
	Name    string `json:"name,omitempty"`    // spawn: session name; create_channel: channel name
	Task    string `json:"task,omitempty"`    // spawn: initial message to inject
	To      string `json:"to,omitempty"`      // send: target session name
	Message string `json:"message,omitempty"` // send: message content
}

// Parse tries to parse line as a Directive. Returns nil, nil if line does not
// begin with '{' (not a JSON object) or has no "type" field. Returns an error
// if the line starts with '{' but is not valid JSON.
func Parse(line string) (*Directive, error) {
	line = strings.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return nil, nil
	}
	var d Directive
	if err := json.Unmarshal([]byte(line), &d); err != nil {
		return nil, err
	}
	if d.Type == "" {
		return nil, nil
	}
	return &d, nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./nova/directive/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add nova/directive/
git commit -m "feat: directive parser for Claude swarm JSON protocol"
```

---

### Task 4: Discord Client Interface, Helpers, and Test Stub

**Goal:** Define a `Client` interface wrapping the discordgo methods Nova needs, implement channel/category management helpers, and create a fake stub for tests.

**Files:**
- Create: `nova/discord/discord.go`
- Create: `nova/discord/discord_test.go`
- Create: `internal/testdiscord/testdiscord.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/discord/... -v` passes
- [ ] `EnsureCategory` returns existing category rather than creating a duplicate
- [ ] `EnsureChannel` returns existing channel rather than creating a duplicate
- [ ] `ArchiveChannel` renames channel with `✓-` prefix
- [ ] `PostMessage` splits content longer than 2000 chars into multiple messages

**Verify:** `go test ./nova/discord/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write testdiscord stub**

Create `internal/testdiscord/testdiscord.go`:

```go
// Package testdiscord provides a fake discord.Client for use in tests.
package testdiscord

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

// Message records a message sent via ChannelMessageSend.
type Message struct {
	ChannelID string
	Content   string
}

// Session is a fake discord.Client.
type Session struct {
	mu       sync.Mutex
	channels map[string]*discordgo.Channel
	Messages []Message
	idSeq    atomic.Int64
}

// New returns a new fake Session.
func New() *Session {
	return &Session{channels: make(map[string]*discordgo.Channel)}
}

func (s *Session) nextID() string {
	return fmt.Sprintf("fake-%d", s.idSeq.Add(1))
}

func (s *Session) GuildChannels(guildID string) ([]*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*discordgo.Channel
	for _, ch := range s.channels {
		if ch.GuildID == guildID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (s *Session) GuildChannelCreateComplex(guildID string, data discordgo.GuildChannelCreateData) (*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := &discordgo.Channel{
		ID:       s.nextID(),
		GuildID:  guildID,
		Name:     data.Name,
		Type:     data.Type,
		ParentID: data.ParentID,
		Topic:    data.Topic,
	}
	s.channels[ch.ID] = ch
	return ch, nil
}

func (s *Session) ChannelEdit(channelID string, data *discordgo.ChannelEdit) (*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel %s not found", channelID)
	}
	if data.Name != "" {
		ch.Name = data.Name
	}
	if data.ParentID != "" {
		ch.ParentID = data.ParentID
	}
	if data.Topic != nil {
		ch.Topic = *data.Topic
	}
	return ch, nil
}

func (s *Session) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, Message{ChannelID: channelID, Content: content})
	return &discordgo.Message{ID: s.nextID()}, nil
}

func (s *Session) ChannelPermissionSet(channelID, targetID string, targetType discordgo.PermissionOverwriteType, allow, deny int64) error {
	return nil
}

// GetChannel returns a channel by ID (test helper).
func (s *Session) GetChannel(id string) (*discordgo.Channel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	return ch, ok
}
```

- [ ] **Step 2: Write discord_test.go**

Create `nova/discord/discord_test.go`:

```go
package discord_test

import (
	"strings"
	"testing"

	"nova/internal/testdiscord"
	"nova/nova/discord"
)

const guildID = "guild-1"

func TestEnsureCategory_creates(t *testing.T) {
	fake := testdiscord.New()
	id, err := discord.EnsureCategory(fake, guildID, "Nova: solo")
	if err != nil {
		t.Fatalf("EnsureCategory: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty category ID")
	}
}

func TestEnsureCategory_idempotent(t *testing.T) {
	fake := testdiscord.New()
	id1, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	id2, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	if id1 != id2 {
		t.Errorf("EnsureCategory created duplicate: %q vs %q", id1, id2)
	}
}

func TestEnsureChannel_creates(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	chID, err := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if chID == "" {
		t.Error("expected non-empty channel ID")
	}
}

func TestEnsureChannel_idempotent(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	id1, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	id2, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	if id1 != id2 {
		t.Errorf("EnsureChannel created duplicate: %q vs %q", id1, id2)
	}
}

func TestArchiveChannel(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	archID, _ := discord.EnsureCategory(fake, guildID, "Nova: archived")
	chID, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")

	if err := discord.ArchiveChannel(fake, guildID, chID, archID); err != nil {
		t.Fatalf("ArchiveChannel: %v", err)
	}
	ch, ok := fake.GetChannel(chID)
	if !ok {
		t.Fatal("channel not found after archive")
	}
	if !strings.HasPrefix(ch.Name, "✓-") {
		t.Errorf("channel name = %q, want ✓- prefix", ch.Name)
	}
	if ch.ParentID != archID {
		t.Errorf("ParentID = %q, want %q", ch.ParentID, archID)
	}
}

func TestPostMessage_short(t *testing.T) {
	fake := testdiscord.New()
	if err := discord.PostMessage(fake, "ch-1", "hello"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(fake.Messages) != 1 {
		t.Errorf("got %d messages, want 1", len(fake.Messages))
	}
}

func TestPostMessage_splits(t *testing.T) {
	fake := testdiscord.New()
	long := strings.Repeat("x", 4001)
	if err := discord.PostMessage(fake, "ch-1", long); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(fake.Messages) != 3 {
		t.Errorf("got %d messages, want 3", len(fake.Messages))
	}
}
```

- [ ] **Step 3: Run test — expect compile error**

```bash
go test ./nova/discord/... -v
```

- [ ] **Step 4: Write nova/discord/discord.go**

```go
// Package discord provides channel and category management helpers for Nova.
package discord

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Client wraps the discordgo methods used by Nova.
// *discordgo.Session satisfies this interface directly.
type Client interface {
	GuildChannels(guildID string) ([]*discordgo.Channel, error)
	GuildChannelCreateComplex(guildID string, data discordgo.GuildChannelCreateData) (*discordgo.Channel, error)
	ChannelEdit(channelID string, data *discordgo.ChannelEdit) (*discordgo.Channel, error)
	ChannelMessageSend(channelID, content string) (*discordgo.Message, error)
	ChannelPermissionSet(channelID, targetID string, targetType discordgo.PermissionOverwriteType, allow, deny int64) error
}

// EnsureCategory returns the ID of the category named name in guildID,
// creating it if it does not exist.
func EnsureCategory(c Client, guildID, name string) (string, error) {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return "", fmt.Errorf("GuildChannels: %w", err)
	}
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildCategory && ch.Name == name {
			return ch.ID, nil
		}
	}
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name: name,
		Type: discordgo.ChannelTypeGuildCategory,
	})
	if err != nil {
		return "", fmt.Errorf("create category %q: %w", name, err)
	}
	return ch.ID, nil
}

// EnsureChannel returns the ID of the text channel named name in categoryID,
// creating it if it does not exist.
func EnsureChannel(c Client, guildID, categoryID, name string) (string, error) {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return "", fmt.Errorf("GuildChannels: %w", err)
	}
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildText && ch.ParentID == categoryID && ch.Name == name {
			return ch.ID, nil
		}
	}
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: categoryID,
	})
	if err != nil {
		return "", fmt.Errorf("create channel %q: %w", name, err)
	}
	return ch.ID, nil
}

// CreateChannel creates a new text channel in categoryID and returns its ID.
// Unlike EnsureChannel, this always creates a new channel.
func CreateChannel(c Client, guildID, categoryID, name string) (string, error) {
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: categoryID,
	})
	if err != nil {
		return "", fmt.Errorf("create channel %q: %w", name, err)
	}
	return ch.ID, nil
}

// ArchiveChannel renames channelID to "✓-<name>", moves it to
// archiveCategoryID, and makes it read-only.
func ArchiveChannel(c Client, guildID, channelID, archiveCategoryID string) error {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return err
	}
	var current *discordgo.Channel
	for _, ch := range channels {
		if ch.ID == channelID {
			current = ch
			break
		}
	}
	if current == nil {
		return fmt.Errorf("channel %s not found", channelID)
	}

	newName := "✓-" + strings.TrimPrefix(current.Name, "✓-")
	if _, err := c.ChannelEdit(channelID, &discordgo.ChannelEdit{
		Name:     newName,
		ParentID: archiveCategoryID,
	}); err != nil {
		return fmt.Errorf("edit channel: %w", err)
	}

	// Deny SEND_MESSAGES for @everyone (targetID = guildID for @everyone role).
	const sendMessages = 0x0000000000000800
	if err := c.ChannelPermissionSet(channelID, guildID, discordgo.PermissionOverwriteTypeRole, 0, sendMessages); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	return nil
}

// SetChannelTopic updates a channel's topic string.
func SetChannelTopic(c Client, channelID, topic string) error {
	_, err := c.ChannelEdit(channelID, &discordgo.ChannelEdit{Topic: &topic})
	return err
}

// PostMessage sends content to channelID, splitting into 2000-char chunks.
func PostMessage(c Client, channelID, content string) error {
	const limit = 2000
	for len(content) > 0 {
		chunk := content
		if len(chunk) > limit {
			chunk = content[:limit]
		}
		content = content[len(chunk):]
		if _, err := c.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run tests — expect pass**

```bash
go test ./nova/discord/... ./internal/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 6: Commit**

```bash
git add nova/discord/ internal/testdiscord/
git commit -m "feat: discord client interface, helpers, and test stub"
```

---

### Task 5: Session Subprocess

**Goal:** `Session` struct with hot/cold state machine, subprocess I/O goroutines, directive dispatch, and idle timer. Uses findings from Task 0 spike for claude flags.

**Files:**
- Create: `nova/session/session.go`
- Create: `nova/session/session_test.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/session/... -v` passes
- [ ] `Warm` starts a subprocess and transitions status to `hot`
- [ ] `Send` delivers messages to subprocess stdin
- [ ] Responses accumulate until `{"type":"done"}` then `OnContent` is called
- [ ] Directives intercepted from stdout call `OnDirective`, not `OnContent`
- [ ] Idle timer fires `OnIdle` after timeout
- [ ] `Terminate` stops subprocess and sets status to `terminated`
- [ ] `cool` (idle timer expiry) sets status to `cold`

**Verify:** `go test ./nova/session/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write session_test.go**

Create `nova/session/session_test.go`:

```go
package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nova/nova/directive"
	"nova/nova/session"
)

// fakeClaude writes a shell script that echoes each stdin line back with
// a {"type":"done"} sentinel, simulating Claude's response protocol.
func fakeClaude(t *testing.T) string {
	t.Helper()
	script := "#!/bin/sh\nwhile IFS= read -r line; do\n  printf '%s\\n' \"$line\"\n  printf '{\"type\":\"done\"}\\n'\ndone\n"
	path := filepath.Join(t.TempDir(), "fakeclaude")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSession_WarmAndSend(t *testing.T) {
	bin := fakeClaude(t)
	contentCh := make(chan string, 1)

	s := session.New("id-1", "worker", t.TempDir(), "ch-1", "")
	err := s.Warm(context.Background(), bin, "", 30*time.Second, session.Callbacks{
		OnContent:   func(_, content string) { contentCh <- content },
		OnDirective: func(_ *session.Session, _ directive.Directive) {},
		OnIdle:      func(_ string) {},
	})
	if err != nil {
		t.Fatalf("Warm: %v", err)
	}
	if s.Status != session.StatusHot {
		t.Errorf("Status = %q, want hot", s.Status)
	}

	if err := s.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case got := <-contentCh:
		if got != "hello" {
			t.Errorf("content = %q, want %q", got, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for content")
	}
}

func TestSession_DirectiveIntercepted(t *testing.T) {
	// Script emits a directive then done — directive should NOT appear in content.
	script := "#!/bin/sh\nprintf '{\"type\":\"spawn\",\"name\":\"w\",\"task\":\"t\"}\\n'\nprintf '{\"type\":\"done\"}\\n'\ncat\n"
	path := filepath.Join(t.TempDir(), "fakeclaude")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	contentCh := make(chan string, 1)
	dirCh := make(chan directive.Directive, 1)

	s := session.New("id-2", "orch", t.TempDir(), "ch-2", "")
	_ = s.Warm(context.Background(), path, "", 30*time.Second, session.Callbacks{
		OnContent:   func(_, content string) { contentCh <- content },
		OnDirective: func(_ *session.Session, d directive.Directive) { dirCh <- d },
		OnIdle:      func(_ string) {},
	})

	// Wait for directive
	select {
	case d := <-dirCh:
		if d.Type != directive.TypeSpawn {
			t.Errorf("directive type = %q, want spawn", d.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for directive")
	}

	// Content should be empty (no non-directive lines before done)
	select {
	case got := <-contentCh:
		if got != "" {
			t.Errorf("expected empty content, got %q", got)
		}
	default:
		// good — no content posted
	}
}

func TestSession_IdleTimer(t *testing.T) {
	bin := fakeClaude(t)
	idleCh := make(chan string, 1)

	s := session.New("id-3", "idle-test", t.TempDir(), "ch-3", "")
	_ = s.Warm(context.Background(), bin, "", 100*time.Millisecond, session.Callbacks{
		OnContent:   func(_, _ string) {},
		OnDirective: func(_ *session.Session, _ directive.Directive) {},
		OnIdle:      func(id string) { idleCh <- id },
	})

	select {
	case id := <-idleCh:
		if id != "id-3" {
			t.Errorf("OnIdle id = %q, want id-3", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle timer never fired")
	}

	if s.Status != session.StatusCold {
		t.Errorf("Status after idle = %q, want cold", s.Status)
	}
}

func TestSession_Terminate(t *testing.T) {
	bin := fakeClaude(t)
	s := session.New("id-4", "term", t.TempDir(), "ch-4", "")
	_ = s.Warm(context.Background(), bin, "", 30*time.Second, session.Callbacks{
		OnContent:   func(_, _ string) {},
		OnDirective: func(_ *session.Session, _ directive.Directive) {},
		OnIdle:      func(_ string) {},
	})
	s.Terminate()
	if s.Status != session.StatusTerminated {
		t.Errorf("Status = %q, want terminated", s.Status)
	}
	if err := s.Send("hello"); err == nil {
		t.Error("expected error sending to terminated session")
	}
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
go test ./nova/session/... -v
```

- [ ] **Step 3: Write nova/session/session.go**

```go
// Package session manages Claude Code subprocess sessions.
package session

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"nova/nova/directive"
)

const (
	StatusHot        = "hot"
	StatusCold       = "cold"
	StatusTerminated = "terminated"
)

// Callbacks holds functions the session calls during operation.
type Callbacks struct {
	// OnContent is called with the accumulated response when {"type":"done"} is received.
	OnContent func(channelID, content string)
	// OnDirective is called for each non-done directive line intercepted from stdout.
	OnDirective func(sess *Session, d directive.Directive)
	// OnIdle is called when the idle timer fires, with the session ID.
	OnIdle func(sessID string)
}

// Session represents one Claude Code instance.
type Session struct {
	ID        string
	Name      string
	ClaudeSID string
	Workspace string
	ChannelID string
	SwarmID   string
	Status    string

	mu        sync.Mutex
	callbacks Callbacks
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	msgCh     chan string
}

// New creates a cold Session with the given parameters.
func New(id, name, workspace, channelID, swarmID string) *Session {
	return &Session{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		ChannelID: channelID,
		SwarmID:   swarmID,
		Status:    StatusCold,
	}
}

// Warm starts the Claude subprocess and transitions the session to hot.
// claudeBin is the path to the claude binary. systemPromptPath is written
// via --system-prompt-file; pass empty string to skip (e.g. in tests).
// Adjust flags based on Task 0 spike findings.
func (s *Session) Warm(ctx context.Context, claudeBin, systemPromptPath string, idleTimeout time.Duration, cb Callbacks) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusHot {
		return nil
	}
	if s.Status == StatusTerminated {
		return fmt.Errorf("session %s is terminated", s.ID)
	}

	args := buildArgs(s.ClaudeSID, systemPromptPath)
	cmd := exec.CommandContext(ctx, claudeBin, args...)
	cmd.Dir = s.Workspace

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdoutPipe)
	s.msgCh = make(chan string, 8)
	s.callbacks = cb
	s.Status = StatusHot

	go s.readLoop()
	go s.writeLoop(idleTimeout)

	return nil
}

// buildArgs constructs the claude CLI argument list.
// Update this function based on Task 0 spike findings.
func buildArgs(claudeSID, systemPromptPath string) []string {
	var args []string
	if claudeSID != "" {
		args = append(args, "--resume", claudeSID)
	}
	if systemPromptPath != "" {
		args = append(args, "--system-prompt-file", systemPromptPath)
	}
	return args
}

// Send delivers a message to the Claude subprocess stdin.
// Returns an error if the session is not hot or the buffer is full.
func (s *Session) Send(msg string) error {
	s.mu.Lock()
	ch := s.msgCh
	status := s.Status
	s.mu.Unlock()

	if status != StatusHot {
		return fmt.Errorf("session %q is %s, not hot", s.Name, status)
	}
	select {
	case ch <- msg:
		return nil
	default:
		return fmt.Errorf("session %q message buffer full", s.Name)
	}
}

// Terminate stops the subprocess and marks the session terminated.
func (s *Session) Terminate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopSubprocess()
	s.Status = StatusTerminated
}

// stopSubprocess kills the process and clears I/O fields. Must be called with s.mu held.
func (s *Session) stopSubprocess() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGTERM)
		s.cmd.Wait()
		s.cmd = nil
	}
	if s.stdin != nil {
		s.stdin.Close()
		s.stdin = nil
	}
	s.stdout = nil
	if s.msgCh != nil {
		close(s.msgCh)
		s.msgCh = nil
	}
}

// cool transitions a hot session to cold. Called by idle timer or when stdout closes.
func (s *Session) cool() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusHot {
		return
	}
	s.stopSubprocess()
	s.Status = StatusCold
}

// readLoop reads stdout line-by-line, dispatching directives and accumulating
// content until {"type":"done"} is received.
func (s *Session) readLoop() {
	var buf strings.Builder
	for {
		s.mu.Lock()
		stdout := s.stdout
		s.mu.Unlock()
		if stdout == nil {
			break
		}

		line, err := stdout.ReadString('\n')
		if err != nil {
			// Subprocess closed stdout — flush any remaining content and go cold.
			if content := strings.TrimSpace(buf.String()); content != "" {
				s.callbacks.OnContent(s.ChannelID, content)
			}
			s.cool()
			return
		}

		trimmed := strings.TrimRight(line, "\n\r")
		d, parseErr := directive.Parse(trimmed)
		if parseErr != nil {
			// Malformed JSON that starts with '{' — treat as content.
			buf.WriteString(line)
			continue
		}
		if d != nil {
			switch d.Type {
			case directive.TypeDone:
				if content := strings.TrimSpace(buf.String()); content != "" {
					s.callbacks.OnContent(s.ChannelID, content)
				}
				buf.Reset()
			default:
				s.callbacks.OnDirective(s, *d)
			}
			continue
		}
		buf.WriteString(line)
	}
}

// writeLoop drains msgCh and writes messages to stdin, resetting the idle timer.
func (s *Session) writeLoop(idleTimeout time.Duration) {
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	s.mu.Lock()
	ch := s.msgCh
	s.mu.Unlock()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)
			s.mu.Lock()
			stdin := s.stdin
			s.mu.Unlock()
			if stdin == nil {
				return
			}
			fmt.Fprintln(stdin, msg)

		case <-timer.C:
			s.callbacks.OnIdle(s.ID)
			s.cool()
			return
		}
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./nova/session/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add nova/session/session.go nova/session/session_test.go
git commit -m "feat: session subprocess with hot/cold state machine and directive I/O"
```

---

### Task 6: Session Manager

**Goal:** `Manager` that owns the in-memory session map, handles spawn/kill/resume, captures Claude session IDs, and wires directive dispatch.

**Files:**
- Create: `nova/session/manager.go`
- Create: `nova/session/manager_test.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/session/... -v` continues to pass
- [ ] `Spawn` creates workspace, Discord channel, DB record, and starts subprocess
- [ ] `Kill` terminates session, archives Discord channel, updates DB
- [ ] `ByChannel` returns the session for a given Discord channel ID
- [ ] `WarmIfCold` warms a cold session before routing a message to it
- [ ] On idle callback, session status updated in DB to cold

**Verify:** `go test ./nova/session/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write manager_test.go**

Create `nova/session/manager_test.go`:

```go
package session_test

import (
	"context"
	"testing"
	"time"

	"nova/config"
	"nova/internal/testdiscord"
	"nova/nova/db"
	"nova/nova/session"
)

func newTestManager(t *testing.T) (*session.Manager, *db.Store, *testdiscord.Session) {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	fake := testdiscord.New()
	cfg := &config.Config{
		GuildID:            "g-1",
		SessionRoot:        t.TempDir(),
		IdleTimeoutMinutes: 1,
		ClaudeBin:          fakeClaude(t),
	}
	mgr := session.NewManager(store, fake, cfg, "cat-solo", "cat-archive")
	return mgr, store, fake
}

func TestManager_Spawn(t *testing.T) {
	mgr, store, fake := newTestManager(t)

	sess, err := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "alpha"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sess.Status != session.StatusHot {
		t.Errorf("Status = %q, want hot", sess.Status)
	}
	// Verify DB record
	dbSess, err := store.GetSessionByName("alpha")
	if err != nil {
		t.Fatalf("GetSessionByName: %v", err)
	}
	if dbSess.Status != session.StatusHot {
		t.Errorf("DB status = %q, want hot", dbSess.Status)
	}
	// Verify Discord channel created
	if len(fake.Messages) == 0 && sess.ChannelID == "" {
		t.Error("expected Discord channel to be created")
	}
}

func TestManager_ByChannel(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "beta"})
	got := mgr.ByChannel(sess.ChannelID)
	if got == nil || got.ID != sess.ID {
		t.Errorf("ByChannel returned wrong session")
	}
}

func TestManager_Kill(t *testing.T) {
	mgr, store, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "gamma"})

	if err := mgr.Kill("gamma"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if sess.Status != session.StatusTerminated {
		t.Errorf("Status = %q, want terminated", sess.Status)
	}
	dbSess, _ := store.GetSessionByName("gamma")
	if dbSess.Status != session.StatusTerminated {
		t.Errorf("DB status = %q, want terminated", dbSess.Status)
	}
}

func TestManager_WarmIfCold(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "delta"})

	// Force cold
	sess.Terminate()
	// Reset to cold manually for test
	sess.Status = session.StatusCold

	if err := mgr.WarmIfCold(context.Background(), sess.ID); err != nil {
		t.Fatalf("WarmIfCold: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // let subprocess start
	if sess.Status != session.StatusHot {
		t.Errorf("Status after WarmIfCold = %q, want hot", sess.Status)
	}
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
go test ./nova/session/... -v
```

- [ ] **Step 3: Write nova/session/manager.go**

```go
package session

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nova/config"
	"nova/nova/db"
	"nova/nova/directive"
	discordhelper "nova/nova/discord"

	"github.com/google/uuid"
)

// SpawnOpts carries options for creating a new session.
type SpawnOpts struct {
	Name       string // auto-generated if empty
	SwarmID    string
	Task       string // injected as the first message if non-empty
	CategoryID string // Discord category; uses soloCategoryID if empty
}

// Manager owns all active sessions and handles their lifecycle.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session // id → session
	byName          map[string]*Session
	byChan          map[string]*Session // channelID → session

	store           *db.Store
	discord         discordhelper.Client
	cfg             *config.Config
	soloCategoryID  string
	archiveCategoryID string
}

// NewManager creates a Manager. soloCategoryID and archiveCategoryID are the
// Discord category IDs for solo sessions and archived sessions respectively.
func NewManager(store *db.Store, discord discordhelper.Client, cfg *config.Config, soloCategoryID, archiveCategoryID string) *Manager {
	return &Manager{
		sessions:          make(map[string]*Session),
		byName:            make(map[string]*Session),
		byChan:            make(map[string]*Session),
		store:             store,
		discord:           discord,
		cfg:               cfg,
		soloCategoryID:    soloCategoryID,
		archiveCategoryID: archiveCategoryID,
	}
}

// Spawn creates a new session: workspace, Discord channel, DB record, subprocess.
func (m *Manager) Spawn(ctx context.Context, opts SpawnOpts) (*Session, error) {
	id := uuid.New().String()
	name := opts.Name
	if name == "" {
		name = generateName()
	}

	workspace := filepath.Join(m.cfg.SessionRoot, id)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	catID := opts.CategoryID
	if catID == "" {
		catID = m.soloCategoryID
	}

	channelID, err := discordhelper.CreateChannel(m.discord, m.cfg.GuildID, catID, name)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	dbSess := db.Session{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		ChannelID: channelID,
		SwarmID:   opts.SwarmID,
		Status:    StatusCold,
	}
	if err := m.store.CreateSession(dbSess); err != nil {
		return nil, fmt.Errorf("create session record: %w", err)
	}

	sess := New(id, name, workspace, channelID, opts.SwarmID)

	m.mu.Lock()
	m.sessions[id] = sess
	m.byName[name] = sess
	m.byChan[channelID] = sess
	m.mu.Unlock()

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
		return nil, fmt.Errorf("warm: %w", err)
	}
	if err := m.store.UpdateSessionStatus(id, StatusHot); err != nil {
		return nil, err
	}

	if opts.Task != "" {
		if err := sess.Send(opts.Task); err != nil {
			return nil, fmt.Errorf("send initial task: %w", err)
		}
	}

	return sess, nil
}

// Kill terminates a session by name and archives its Discord channel.
func (m *Manager) Kill(name string) error {
	m.mu.RLock()
	sess := m.byName[name]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %q not found", name)
	}

	sess.Terminate()

	if err := m.store.UpdateSessionStatus(sess.ID, StatusTerminated); err != nil {
		return err
	}

	return discordhelper.ArchiveChannel(m.discord, m.cfg.GuildID, sess.ChannelID, m.archiveCategoryID)
}

// WarmIfCold warms a cold session by ID. No-op if already hot.
func (m *Manager) WarmIfCold(ctx context.Context, id string) error {
	m.mu.RLock()
	sess := m.sessions[id]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %s not found", id)
	}
	if sess.Status == StatusHot {
		return nil
	}
	// Reload ClaudeSID from DB in case it was updated.
	dbSess, err := m.store.GetSession(id)
	if err != nil {
		return err
	}
	sess.ClaudeSID = dbSess.ClaudeSID

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	return sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx))
}

// ByChannel returns the session whose Discord channel matches channelID, or nil.
func (m *Manager) ByChannel(channelID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byChan[channelID]
}

// ByName returns the session with the given name, or nil.
func (m *Manager) ByName(name string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byName[name]
}

// List returns all active sessions, optionally filtered by swarm ID.
func (m *Manager) List(swarmID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Session
	for _, s := range m.sessions {
		if s.Status == StatusTerminated {
			continue
		}
		if swarmID != "" && s.SwarmID != swarmID {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *Manager) makeCallbacks(ctx context.Context) Callbacks {
	return Callbacks{
		OnContent: func(channelID, content string) {
			_ = discordhelper.PostMessage(m.discord, channelID, content)
		},
		OnDirective: func(sess *Session, d directive.Directive) {
			m.handleDirective(ctx, sess, d)
		},
		OnIdle: func(sessID string) {
			_ = m.store.UpdateSessionStatus(sessID, StatusCold)
		},
	}
}

func (m *Manager) handleDirective(ctx context.Context, src *Session, d directive.Directive) {
	switch d.Type {
	case directive.TypeSpawn:
		catID := m.soloCategoryID
		if src.SwarmID != "" {
			if sw, err := m.store.GetSwarm(src.SwarmID); err == nil {
				catID = sw.CategoryID
			}
		}
		_, _ = m.Spawn(ctx, SpawnOpts{
			Name:       d.Name,
			SwarmID:    src.SwarmID,
			Task:       d.Task,
			CategoryID: catID,
		})

	case directive.TypeSend:
		m.mu.RLock()
		target := m.byName[d.To]
		m.mu.RUnlock()
		if target == nil {
			return
		}
		_ = m.WarmIfCold(ctx, target.ID)
		_ = target.Send(d.Message)

	case directive.TypeCreateChannel:
		catID := m.soloCategoryID
		if src.SwarmID != "" {
			if sw, err := m.store.GetSwarm(src.SwarmID); err == nil {
				catID = sw.CategoryID
			}
		}
		_, _ = discordhelper.CreateChannel(m.discord, m.cfg.GuildID, catID, d.Name)
	}
}

// systemPromptPath returns ~/.nova/system-prompt.txt.
func systemPromptPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nova", "system-prompt.txt")
}

var adjectives = []string{"amber", "bold", "calm", "deft", "keen", "sage", "swift", "vast", "wry", "zeal"}
var nouns = []string{"atlas", "bloom", "crane", "drift", "ember", "flint", "grove", "haven", "isle", "jade"}

func generateName() string {
	return adjectives[rand.Intn(len(adjectives))] + "-" + nouns[rand.Intn(len(nouns))]
}
```

- [ ] **Step 4: Add Claude session ID capture to Spawn()**

After `sess.Warm()` succeeds in `Spawn()`, add a goroutine that captures the Claude session ID using directory diffing. This is the default approach — revise the mechanism based on Task 0 spike findings.

Add this helper to `manager.go`:

```go
// captureClaudeSID attempts to find the newest entry in ~/.claude/projects/
// that appeared after beforeTime and stores it in the DB and session.
// This is a best-effort heuristic; update based on Task 0 spike findings.
func (m *Manager) captureClaudeSID(sessID string, sess *Session, beforeTime time.Time) {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	time.Sleep(2 * time.Second) // allow claude to create its session directory

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}
	var newest string
	var newestMod time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.ModTime().After(beforeTime) {
			continue
		}
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
			newest = e.Name()
		}
	}
	if newest == "" {
		return
	}
	_ = m.store.UpdateSessionClaudeSID(sessID, newest)
	sess.mu.Lock()
	sess.ClaudeSID = newest
	sess.mu.Unlock()
}
```

Then in `Spawn()`, just after the `sess.Warm(...)` call:

```go
spawnTime := time.Now()
if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
    return nil, fmt.Errorf("warm: %w", err)
}
go m.captureClaudeSID(id, sess, spawnTime)
```

- [ ] **Step 5: Run tests — expect pass**

```bash
go test ./nova/session/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 6: Commit**

```bash
git add nova/session/manager.go nova/session/manager_test.go
git commit -m "feat: session manager with spawn, kill, warm, directive routing, and session ID capture"
```

---

### Task 7: Swarm Manager

**Goal:** `swarm.Manager` that creates and dissolves swarms (Discord categories + DB records) and broadcasts messages.

**Files:**
- Create: `nova/swarm/swarm.go`
- Create: `nova/swarm/swarm_test.go`

**Acceptance Criteria:**
- [ ] `go test ./nova/swarm/... -v` passes
- [ ] `Create` inserts DB record and creates Discord category
- [ ] `Dissolve` kills all sessions in swarm, archives channels, deletes DB record
- [ ] `Broadcast` sends message to all non-terminated sessions in swarm

**Verify:** `go test ./nova/swarm/... -v` → all tests pass

**Steps:**

- [ ] **Step 1: Write swarm_test.go**

Create `nova/swarm/swarm_test.go`:

```go
package swarm_test

import (
	"context"
	"testing"

	"nova/config"
	"nova/internal/testdiscord"
	"nova/nova/db"
	"nova/nova/session"
	"nova/nova/swarm"
)

func setup(t *testing.T) (*swarm.Manager, *db.Store, *testdiscord.Session) {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	fake := testdiscord.New()
	cfg := &config.Config{GuildID: "g-1", SessionRoot: t.TempDir(), IdleTimeoutMinutes: 1, ClaudeBin: "true"}
	sessMgr := session.NewManager(store, fake, cfg, "cat-solo", "cat-arch")
	mgr := swarm.NewManager(store, fake, sessMgr, "g-1")
	return mgr, store, fake
}

func TestSwarm_Create(t *testing.T) {
	mgr, store, _ := setup(t)
	sw, err := mgr.Create("backend")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sw.Name != "backend" {
		t.Errorf("Name = %q, want backend", sw.Name)
	}
	dbSw, err := store.GetSwarmByName("backend")
	if err != nil {
		t.Fatalf("GetSwarmByName: %v", err)
	}
	if dbSw.CategoryID == "" {
		t.Error("CategoryID should be set")
	}
}

func TestSwarm_CreateDuplicate(t *testing.T) {
	mgr, _, _ := setup(t)
	_, _ = mgr.Create("frontend")
	_, err := mgr.Create("frontend")
	if err == nil {
		t.Error("expected error for duplicate swarm name")
	}
}

func TestSwarm_Broadcast(t *testing.T) {
	mgr, _, fake := setup(t)
	sw, _ := mgr.Create("team")
	_ = sw

	// Broadcast with no sessions should succeed (no-op).
	if err := mgr.Broadcast(context.Background(), "team", "hello swarm"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	_ = fake
}
```

- [ ] **Step 2: Run test — expect compile error**

```bash
go test ./nova/swarm/... -v
```

- [ ] **Step 3: Write nova/swarm/swarm.go**

```go
// Package swarm manages groups of Nova sessions.
package swarm

import (
	"context"
	"fmt"

	discordhelper "nova/nova/discord"
	"nova/nova/db"
	"nova/nova/session"

	"github.com/google/uuid"
)

// Manager creates and manages swarms.
type Manager struct {
	store    *db.Store
	discord  discordhelper.Client
	sessions *session.Manager
	guildID  string
}

// NewManager creates a swarm Manager.
func NewManager(store *db.Store, discord discordhelper.Client, sessions *session.Manager, guildID string) *Manager {
	return &Manager{store: store, discord: discord, sessions: sessions, guildID: guildID}
}

// Create creates a new named swarm with a Discord category.
func (m *Manager) Create(name string) (db.Swarm, error) {
	if _, err := m.store.GetSwarmByName(name); err == nil {
		return db.Swarm{}, fmt.Errorf("swarm %q already exists", name)
	}
	catName := "Nova: " + name
	catID, err := discordhelper.EnsureCategory(m.discord, m.guildID, catName)
	if err != nil {
		return db.Swarm{}, fmt.Errorf("create category: %w", err)
	}
	sw := db.Swarm{
		ID:         uuid.New().String(),
		Name:       name,
		CategoryID: catID,
	}
	if err := m.store.CreateSwarm(sw); err != nil {
		return db.Swarm{}, fmt.Errorf("create swarm record: %w", err)
	}
	return sw, nil
}

// Dissolve kills all sessions in the swarm and removes the swarm record.
func (m *Manager) Dissolve(name string) error {
	sw, err := m.store.GetSwarmByName(name)
	if err != nil {
		return fmt.Errorf("swarm %q not found: %w", name, err)
	}
	sessions, err := m.store.ListSessions(sw.ID)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		_ = m.sessions.Kill(s.Name) // best-effort
	}
	return m.store.DeleteSwarm(sw.ID)
}

// Broadcast sends message to all active sessions in the named swarm.
// Cold sessions are warmed before the message is sent.
func (m *Manager) Broadcast(ctx context.Context, swarmName, message string) error {
	sw, err := m.store.GetSwarmByName(swarmName)
	if err != nil {
		return fmt.Errorf("swarm %q not found: %w", swarmName, err)
	}
	dbSessions, err := m.store.ListSessions(sw.ID)
	if err != nil {
		return err
	}
	for _, s := range dbSessions {
		_ = m.sessions.WarmIfCold(ctx, s.ID)
		sess := m.sessions.ByName(s.Name)
		if sess != nil {
			_ = sess.Send(message)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./nova/swarm/... -v
```
Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add nova/swarm/
git commit -m "feat: swarm manager with create, dissolve, and broadcast"
```

---

### Task 8: Discord Message Routing

**Goal:** Route non-bot messages posted in session channels to that session's stdin.

**Files:**
- Modify: `nova/nova/nova.go` (create if not yet existing)

**Acceptance Criteria:**
- [ ] Messages from the bot itself are ignored
- [ ] Messages in channels with no associated session are ignored
- [ ] Messages in session channels are delivered to the session (warming it if cold)

**Steps:**

- [ ] **Step 1: Create nova/nova/nova.go with routing**

```go
// Package nova is the top-level coordinator for the Nova Discord bot.
package nova

import (
	"context"
	"log"

	"nova/nova/session"

	"github.com/bwmarrin/discordgo"
)

// Intents returns the Discord gateway intents Nova requires.
func Intents() discordgo.Intent {
	return discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent
}

// RegisterMessageRouter installs the handler that routes Discord messages to
// the appropriate Claude session's stdin.
func RegisterMessageRouter(dg *discordgo.Session, mgr *session.Manager) {
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore the bot's own messages.
		if m.Author.ID == s.State.User.ID {
			return
		}
		sess := mgr.ByChannel(m.ChannelID)
		if sess == nil {
			return
		}
		ctx := context.Background()
		if sess.Status == session.StatusCold {
			if err := mgr.WarmIfCold(ctx, sess.ID); err != nil {
				log.Printf("WarmIfCold %s: %v", sess.Name, err)
				return
			}
		}
		if err := sess.Send(m.Content); err != nil {
			log.Printf("Send to %s: %v", sess.Name, err)
		}
	})
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add nova/nova/nova.go
git commit -m "feat: Discord message routing to session stdin"
```

---

### Task 9: Slash Commands

**Goal:** Register the `/nova` command tree and implement all interaction handlers.

**Files:**
- Create: `nova/nova/commands/commands.go`

**Acceptance Criteria:**
- [ ] `/nova spawn` creates a session and responds with a channel link
- [ ] `/nova list` responds ephemerally with session table
- [ ] `/nova kill` terminates a session
- [ ] `/nova resume` warms a cold session
- [ ] `/nova status` shows session detail
- [ ] `/nova clean` removes TERMINATED workspaces
- [ ] `/nova swarm create` creates a swarm
- [ ] `/nova swarm dissolve` dissolves a swarm
- [ ] `/nova broadcast` sends to all swarm sessions

**Steps:**

- [ ] **Step 1: Write nova/nova/commands/commands.go**

```go
// Package commands registers and handles Nova slash commands.
package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"nova/nova/db"
	"nova/nova/session"
	"nova/nova/swarm"

	"github.com/bwmarrin/discordgo"
)

type handler struct {
	sessions *session.Manager
	swarms   *swarm.Manager
	store    *db.Store
	guildID  string
}

// Register installs the /nova command and interaction handler on dg.
func Register(dg *discordgo.Session, sessions *session.Manager, swarms *swarm.Manager, store *db.Store, guildID string) {
	h := &handler{sessions: sessions, swarms: swarms, store: store, guildID: guildID}
	dg.AddHandler(h.onInteraction)
}

// RegisterCommands creates the /nova application command. Must be called after
// dg.Open() so dg.State.User is populated.
func RegisterCommands(dg *discordgo.Session, guildID string) error {
	_, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, novaCommand())
	return err
}

func novaCommand() *discordgo.ApplicationCommand {
	str := discordgo.ApplicationCommandOptionString
	sub := discordgo.ApplicationCommandOptionSubCommand
	grp := discordgo.ApplicationCommandOptionSubCommandGroup
	opt := func(name, desc string, typ discordgo.ApplicationCommandOptionType, required bool) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: typ, Name: name, Description: desc, Required: required}
	}
	return &discordgo.ApplicationCommand{
		Name:        "nova",
		Description: "Manage Claude swarm sessions",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: sub, Name: "spawn", Description: "Spawn a new Claude session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name (auto-generated if omitted)", str, false),
				opt("swarm", "Swarm to add this session to", str, false),
			}},
			{Type: sub, Name: "list", Description: "List active sessions", Options: []*discordgo.ApplicationCommandOption{
				opt("swarm", "Filter by swarm name", str, false),
			}},
			{Type: sub, Name: "kill", Description: "Terminate a session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "resume", Description: "Force-warm a cold session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "status", Description: "Show session status", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, false),
			}},
			{Type: sub, Name: "clean", Description: "Delete workspaces of terminated sessions"},
			{Type: sub, Name: "broadcast", Description: "Send message to all sessions in a swarm", Options: []*discordgo.ApplicationCommandOption{
				opt("swarm", "Swarm name", str, true),
				opt("message", "Message to broadcast", str, true),
			}},
			{Type: grp, Name: "swarm", Description: "Manage swarms", Options: []*discordgo.ApplicationCommandOption{
				{Type: sub, Name: "create", Description: "Create a swarm", Options: []*discordgo.ApplicationCommandOption{
					opt("name", "Swarm name", str, true),
				}},
				{Type: sub, Name: "dissolve", Description: "Dissolve a swarm", Options: []*discordgo.ApplicationCommandOption{
					opt("name", "Swarm name", str, true),
				}},
			}},
		},
	}
}

func (h *handler) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()
	if data.Name != "nova" || len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]
	ctx := context.Background()
	switch sub.Name {
	case "spawn":
		h.handleSpawn(ctx, s, i, sub)
	case "list":
		h.handleList(s, i, sub)
	case "kill":
		h.handleKill(ctx, s, i, sub)
	case "resume":
		h.handleResume(ctx, s, i, sub)
	case "status":
		h.handleStatus(s, i, sub)
	case "clean":
		h.handleClean(s, i)
	case "broadcast":
		h.handleBroadcast(ctx, s, i, sub)
	case "swarm":
		h.handleSwarmGroup(ctx, s, i, sub)
	}
}

func (h *handler) handleSpawn(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name, _ := opts["name"]
	swarmName, _ := opts["swarm"]

	var swarmID string
	if swarmName != "" {
		sw, err := h.store.GetSwarmByName(swarmName)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Swarm %q not found.", swarmName))
			return
		}
		swarmID = sw.ID
	}

	sess, err := h.sessions.Spawn(ctx, session.SpawnOpts{Name: name, SwarmID: swarmID})
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to spawn session: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Spawned **%s** → <#%s>", sess.Name, sess.ChannelID))
}

func (h *handler) handleList(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	swarmName, _ := opts["swarm"]

	var swarmID string
	if swarmName != "" {
		sw, err := h.store.GetSwarmByName(swarmName)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Swarm %q not found.", swarmName))
			return
		}
		swarmID = sw.ID
	}

	sessions, err := h.store.ListSessions(swarmID)
	if err != nil {
		respondEphemeral(s, i, "Error fetching sessions.")
		return
	}
	if len(sessions) == 0 {
		respondEphemeral(s, i, "No active sessions.")
		return
	}
	var sb strings.Builder
	sb.WriteString("```\nName            Status  Swarm           Last Active\n")
	sb.WriteString("──────────────────────────────────────────────────────\n")
	for _, sess := range sessions {
		sb.WriteString(fmt.Sprintf("%-16s%-8s%-16s%s\n",
			truncate(sess.Name, 15),
			sess.Status,
			truncate(sess.SwarmID, 15),
			sess.LastActive.Format(time.RFC822),
		))
	}
	sb.WriteString("```")
	respondEphemeral(s, i, sb.String())
}

func (h *handler) handleKill(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name := opts["name"]
	if err := h.sessions.Kill(name); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Kill failed: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Session **%s** terminated.", name))
}

func (h *handler) handleResume(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name := opts["name"]
	sess := h.sessions.ByName(name)
	if sess == nil {
		respondEphemeral(s, i, fmt.Sprintf("Session %q not found.", name))
		return
	}
	if err := h.sessions.WarmIfCold(ctx, sess.ID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Resume failed: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Session **%s** is now hot.", name))
}

func (h *handler) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name, _ := opts["name"]
	var dbSess db.Session
	var err error
	if name != "" {
		dbSess, err = h.store.GetSessionByName(name)
	} else {
		respondEphemeral(s, i, "Specify a session name.")
		return
	}
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Session %q not found.", name))
		return
	}
	n, _ := h.store.CountMessages(dbSess.ID)
	msg := fmt.Sprintf("**%s**\nStatus: `%s`\nWorkspace: `%s`\nChannel: <#%s>\nMessages: %d\nLast active: %s",
		dbSess.Name, dbSess.Status, dbSess.Workspace, dbSess.ChannelID, n,
		dbSess.LastActive.Format(time.RFC1123))
	respondEphemeral(s, i, msg)
}

func (h *handler) handleClean(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sessions, err := h.store.ListTerminatedSessions()
	if err != nil {
		respondEphemeral(s, i, "Error fetching terminated sessions.")
		return
	}
	var cleaned int
	for _, sess := range sessions {
		if err := os.RemoveAll(sess.Workspace); err == nil {
			cleaned++
		}
	}
	respondEphemeral(s, i, fmt.Sprintf("Cleaned %d workspace(s).", cleaned))
}

func (h *handler) handleBroadcast(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	swarmName := opts["swarm"]
	message := opts["message"]
	if err := h.swarms.Broadcast(ctx, swarmName, message); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Broadcast failed: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Broadcast sent to **%s**.", swarmName))
}

func (h *handler) handleSwarmGroup(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	if len(sub.Options) == 0 {
		return
	}
	cmd := sub.Options[0]
	opts := optMap(cmd.Options)
	switch cmd.Name {
	case "create":
		sw, err := h.swarms.Create(opts["name"])
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Create failed: %v", err))
			return
		}
		respondEphemeral(s, i, fmt.Sprintf("Swarm **%s** created.", sw.Name))
	case "dissolve":
		if err := h.swarms.Dissolve(opts["name"]); err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Dissolve failed: %v", err))
			return
		}
		respondEphemeral(s, i, fmt.Sprintf("Swarm **%s** dissolved.", opts["name"]))
	}
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	})
}

func optMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]string {
	m := make(map[string]string)
	for _, o := range opts {
		if o.Value != nil {
			m[o.Name] = fmt.Sprintf("%v", o.Value)
		}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
```

- [ ] **Step 2: Build to verify**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add nova/nova/commands/
git commit -m "feat: /nova slash command tree with all session and swarm handlers"
```

---

### Task 10: Bot Wiring and Startup Sequence

**Goal:** Complete `nova.go` coordinator and `main.go`, write the system prompt file, and implement the full startup sequence from the spec.

**Files:**
- Modify: `nova/nova/nova.go`
- Modify: `main.go`

**Acceptance Criteria:**
- [ ] `go build .` produces working binary
- [ ] Bot refuses to start with empty `bot_token` or `guild_id`
- [ ] On startup: hot sessions reset to cold, system prompt written, control channel ensured, categories ensured, slash command registered
- [ ] `go test ./... -v` — all tests pass

**Verify:** `go build . && go test ./... -v` → all pass, binary produced

**Steps:**

- [ ] **Step 1: Write the complete nova/nova/nova.go**

```go
package nova

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"nova/config"
	"nova/nova/commands"
	"nova/nova/db"
	discordhelper "nova/nova/discord"
	"nova/nova/session"
	"nova/nova/swarm"

	"github.com/bwmarrin/discordgo"
)

const systemPrompt = `You are an agent in a Discord-native swarm. Your responses are posted to a Discord channel.
Always end every response with {"type":"done"} on its own line.

To issue directives to the swarm, emit one JSON object per line with a "type" field.
Directives are intercepted by the bot and not posted to Discord.

Available directive types:
  {"type":"spawn","name":"<name>","task":"<initial message>"}
  {"type":"send","to":"<name>","message":"<msg>"}
  {"type":"create_channel","name":"<name>"}
  {"type":"done"}

All other output is posted to your Discord channel verbatim.`

// Intents returns the Discord gateway intents Nova requires.
func Intents() discordgo.Intent {
	return discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent
}

// Init wires event handlers onto dg. Call before dg.Open().
func Init(dg *discordgo.Session, store *db.Store, cfg *config.Config) (sessionMgr *session.Manager, swarmMgr *swarm.Manager) {
	// Placeholders for category IDs resolved during Run().
	// Message routing and command handlers are registered in Run() after IDs are known.
	return nil, nil
}

// Run performs the startup sequence that requires an open Discord connection.
// Returns initialized managers for use by main.
func Run(ctx context.Context, dg *discordgo.Session, store *db.Store, cfg *config.Config) (*session.Manager, *swarm.Manager, error) {
	// 1. Write system prompt file.
	if err := writeSystemPrompt(); err != nil {
		return nil, nil, fmt.Errorf("write system prompt: %w", err)
	}

	guildID := cfg.GuildID

	// 2. Ensure control channel.
	_, err := discordhelper.EnsureChannel(dg, guildID, "", cfg.ControlChannelName)
	if err != nil {
		log.Printf("warn: could not ensure control channel: %v", err)
	}

	// 3. Ensure fixed categories.
	soloCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: solo")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure solo category: %w", err)
	}
	archiveCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: archived")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure archive category: %w", err)
	}

	// 4. Build managers.
	sessionMgr := session.NewManager(store, dg, cfg, soloCatID, archiveCatID)
	swarmMgr := swarm.NewManager(store, dg, sessionMgr, guildID)

	// 5. Register message router.
	RegisterMessageRouter(dg, sessionMgr)

	// 6. Register slash commands.
	commands.Register(dg, sessionMgr, swarmMgr, store, guildID)
	if err := commands.RegisterCommands(dg, guildID); err != nil {
		return nil, nil, fmt.Errorf("register commands: %w", err)
	}

	log.Println("Nova startup complete.")
	return sessionMgr, swarmMgr, nil
}

func writeSystemPrompt() error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".nova")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "system-prompt.txt")
	return os.WriteFile(path, []byte(systemPrompt), 0o644)
}
```

- [ ] **Step 2: Update main.go to use the full startup sequence**

```go
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"nova/config"
	"nova/nova"
	novadb "nova/nova/db"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.BotToken == "" {
		log.Fatal("bot_token is required in config.toml")
	}
	if cfg.GuildID == "" {
		log.Fatal("guild_id is required in config.toml")
	}

	store, err := novadb.New("data/nova.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	if err := store.ResetActiveSessions(); err != nil {
		log.Fatalf("reset sessions: %v", err)
	}

	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("create discord session: %v", err)
	}
	dg.Identify.Intents = nova.Intents()

	if err := dg.Open(); err != nil {
		log.Fatalf("open discord: %v", err)
	}
	defer dg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, _, err := nova.Run(ctx, dg, store, cfg); err != nil {
		log.Fatalf("nova run: %v", err)
	}

	log.Println("Nova running. Ctrl-C to stop.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
```

- [ ] **Step 3: Build and run all tests**

```bash
go build .
go test ./... -v
```
Expected: binary produced, all tests `PASS`.

- [ ] **Step 4: Final commit**

```bash
git add nova/nova/nova.go main.go
git commit -m "feat: bot wiring, startup sequence, system prompt"
```

---

## Dependency Order

```
Task 0 (spike) → Task 5 (session subprocess) — spike informs CLI flags
Task 1 (scaffold) → all subsequent tasks
Task 2 (db) → Tasks 6, 7, 9
Task 3 (directive) → Task 5
Task 4 (discord) → Tasks 5, 6, 7
Task 5 (session) → Task 6
Task 6 (session manager) → Tasks 7, 8, 9
Task 7 (swarm) → Task 9
Tasks 8+9 → Task 10
```

## Post-Implementation Verification

After Task 10, do a manual smoke test:

1. Copy `config.toml` template and fill in `bot_token` and `guild_id`
2. Run `./nova`
3. Verify `#nova` channel appears in Discord
4. Run `/nova spawn` — verify a channel appears under `[Nova: solo]`
5. Type a message in the new channel — verify Claude responds
6. Wait 10+ minutes — verify channel topic changes to `cold`
7. Type again — verify it warms and responds
8. Run `/nova kill <name>` — verify channel is archived
