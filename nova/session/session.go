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
	gen       int64 // incremented each Warm call; goroutines check before acting
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

	s.gen++
	gen := s.gen
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdoutPipe)
	s.msgCh = make(chan string, 8)
	s.callbacks = cb
	s.Status = StatusHot

	go s.readLoop(gen)
	go s.writeLoop(gen, idleTimeout)

	return nil
}

// buildArgs constructs the claude CLI argument list.
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

// coolIfGen transitions to cold only if gen matches the current generation.
func (s *Session) coolIfGen(gen int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen || s.Status != StatusHot {
		return
	}
	s.stopSubprocess()
	s.Status = StatusCold
}

// readLoop reads stdout line-by-line, dispatching directives and accumulating
// content until {"type":"done"} is received. gen identifies which Warm cycle
// this goroutine belongs to; it exits without acting if a newer cycle started.
func (s *Session) readLoop(gen int64) {
	var buf strings.Builder
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
			// Subprocess closed stdout — flush any remaining content and go cold,
			// but only if we are still the active generation.
			s.mu.Lock()
			isActive := s.gen == gen
			s.mu.Unlock()
			if isActive {
				if content := strings.TrimSpace(buf.String()); content != "" {
					s.callbacks.OnContent(s.ChannelID, content)
				}
				s.coolIfGen(gen)
			}
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
			s.mu.Lock()
			stdin := s.stdin
			s.mu.Unlock()
			if stdin == nil {
				return
			}
			fmt.Fprintln(stdin, msg)

		case <-timer.C:
			s.coolIfGen(gen)
			s.callbacks.OnIdle(s.ID)
			return
		}
	}
}
