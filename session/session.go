// Package session manages Claude Code subprocess sessions.
package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"nova/directive"
)

const (
	StatusHot        = "hot"
	StatusCold       = "cold"
	StatusTerminated = "terminated"
)

// Callbacks holds functions the session calls during operation.
type Callbacks struct {
	// OnTurnStart is called when a message is about to be written to the
	// Claude subprocess stdin — i.e. the moment a turn begins.
	OnTurnStart func(channelID string)
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
	Status    string

	mu        sync.Mutex
	gen       int64 // incremented each Warm call; goroutines check before acting
	callbacks Callbacks
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderrBuf *bytes.Buffer // captures subprocess stderr; read after Wait()
	msgCh     chan string
}

// New creates a cold Session with the given parameters.
func New(id, name, workspace, channelID string) *Session {
	return &Session{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		ChannelID: channelID,
		Status:    StatusCold,
	}
}

// Warm starts the Claude subprocess and transitions the session to hot.
// claudeBin is the path to the claude binary. systemPromptPath is written
// via --system-prompt-file; pass empty string to skip (e.g. in tests).
func (s *Session) Warm(ctx context.Context, claudeBin, systemPromptPath string, idleTimeout time.Duration, cb Callbacks) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusHot {
		return nil
	}
	if s.Status == StatusTerminated {
		return fmt.Errorf("session %s is terminated", s.ID)
	}

	isResume := s.gen > 0
	args := buildArgs(s.ClaudeSID, systemPromptPath, isResume)
	slog.Debug("starting claude subprocess",
		"session", s.Name,
		"bin", claudeBin,
		"workspace", s.Workspace,
		"resume", isResume,
		"idle_timeout", idleTimeout,
	)
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
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}

	s.gen++
	gen := s.gen
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdoutPipe)
	s.stderrBuf = stderrBuf
	s.msgCh = make(chan string, 8)
	s.callbacks = cb
	s.Status = StatusHot
	slog.Debug("claude subprocess started", "session", s.Name, "pid", cmd.Process.Pid, "gen", gen)

	go s.readLoop(gen)
	go s.writeLoop(gen, idleTimeout)

	return nil
}

// buildArgs constructs the claude CLI argument list.
// isResume distinguishes re-warming a cold session (use --resume) from
// starting a brand-new session (use --session-id to pre-assign the UUID).
// The system prompt is only injected on first spawn; resumed sessions already
// carry it in their conversation history.
func buildArgs(claudeSID, systemPromptPath string, isResume bool) []string {
	// --print + stream-json enables non-interactive pipe mode with multi-turn input.
	// --verbose is required by --output-format=stream-json.
	args := []string{
		"--print",
		"--input-format=stream-json",
		"--output-format=stream-json",
		"--verbose",
	}
	if claudeSID != "" {
		if isResume {
			args = append(args, "--resume", claudeSID)
		} else {
			args = append(args, "--session-id", claudeSID)
		}
	}
	if !isResume && systemPromptPath != "" {
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
		if err := s.cmd.Wait(); err != nil {
			stderr := strings.TrimSpace(s.stderrBuf.String())
			slog.Error("claude subprocess exited with error",
				"session", s.Name,
				"err", err,
				"stderr", stderr,
			)
		}
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
	slog.Debug("cooling session", "session", s.Name)
	s.stopSubprocess()
	s.Status = StatusCold
}

// coolIfGen transitions to cold only if gen matches the current generation.
func (s *Session) coolIfGen(gen int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		slog.Debug("coolIfGen: stale generation, ignoring", "session", s.Name, "gen", gen, "current_gen", s.gen)
		return
	}
	if s.Status != StatusHot {
		return
	}
	slog.Debug("cooling session (gen match)", "session", s.Name, "gen", gen)
	s.stopSubprocess()
	s.Status = StatusCold
}

// streamEvent is the minimal shape we need from Claude's stream-json output.
type streamEvent struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// readLoop reads stdout line-by-line, parsing Claude's stream-json events.
// When a "result" event arrives (end of a turn), the result text is scanned
// for embedded directives and the remainder is posted to Discord.
// gen identifies which Warm cycle this goroutine belongs to.
func (s *Session) readLoop(gen int64) {
	for {
		s.mu.Lock()
		stdout := s.stdout
		currentGen := s.gen
		s.mu.Unlock()
		if stdout == nil || currentGen != gen {
			break
		}

		line, err := stdout.ReadString('\n')
		if err != nil {
			// Subprocess closed stdout — cool the session if we're still active.
			s.mu.Lock()
			isActive := s.gen == gen
			s.mu.Unlock()
			if isActive {
				slog.Debug("readLoop: subprocess stdout closed, cooling", "session", s.Name)
				s.coolIfGen(gen)
			}
			return
		}

		trimmed := strings.TrimRight(line, "\n\r")
		if trimmed == "" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
			slog.Debug("readLoop: non-JSON line, ignoring", "session", s.Name)
			continue
		}

		if event.Type != "result" {
			slog.Debug("readLoop: skipping event", "session", s.Name, "type", event.Type)
			continue
		}
		if event.IsError {
			slog.Warn("readLoop: result event is error", "session", s.Name, "result", event.Result)
			continue
		}

		slog.Debug("readLoop: result event", "session", s.Name, "result_len", len(event.Result))
		s.dispatchResult(event.Result)
	}
}

// dispatchResult scans the turn result text for directive lines (JSON starting
// with '{') and posts the remaining content to Discord.
func (s *Session) dispatchResult(text string) {
	var contentBuf strings.Builder
	for _, line := range strings.Split(text, "\n") {
		d, parseErr := directive.Parse(line)
		if parseErr != nil {
			// Malformed JSON starting with '{' — treat as content.
			contentBuf.WriteString(line)
			contentBuf.WriteByte('\n')
			continue
		}
		if d != nil {
			if d.Type != directive.TypeDone {
				slog.Debug("dispatchResult: intercepted directive", "session", s.Name, "type", d.Type)
				s.callbacks.OnDirective(s, *d)
			}
			// TypeDone: no-op — the "result" event already signals turn completion.
			continue
		}
		contentBuf.WriteString(line)
		contentBuf.WriteByte('\n')
	}
	if content := strings.TrimSpace(contentBuf.String()); content != "" {
		slog.Debug("dispatchResult: posting content", "session", s.Name, "content_len", len(content))
		s.callbacks.OnContent(s.ChannelID, content)
	}
}

// writeLoop drains msgCh and writes messages to stdin, resetting the idle timer.
func (s *Session) writeLoop(gen int64, idleTimeout time.Duration) {
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
			slog.Debug("writeLoop: sending message to subprocess", "session", s.Name, "msg_len", len(msg))
			if s.callbacks.OnTurnStart != nil {
				s.callbacks.OnTurnStart(s.ChannelID)
			}
			s.mu.Lock()
			stdin := s.stdin
			s.mu.Unlock()
			if stdin == nil {
				return
			}
			data, err := json.Marshal(map[string]any{
				"type": "user",
				"message": map[string]string{
					"role":    "user",
					"content": msg,
				},
			})
			if err != nil {
				slog.Error("writeLoop: failed to marshal message", "session", s.Name, "err", err)
				continue
			}
			fmt.Fprintln(stdin, string(data))

		case <-timer.C:
			slog.Debug("writeLoop: idle timeout fired", "session", s.Name, "timeout", idleTimeout)
			s.coolIfGen(gen)
			s.callbacks.OnIdle(s.ID)
			return
		}
	}
}
