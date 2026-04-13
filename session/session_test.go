package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nova/directive"
	"nova/session"
)

// fakeClaude writes a shell script that reads stream-json input messages and
// echoes the content back as a stream-json "result" event, simulating Claude's
// --print --input-format=stream-json --output-format=stream-json protocol.
func fakeClaude(t *testing.T) string {
	t.Helper()
	// Extract content from {"type":"user","message":{"role":"user","content":"<msg>"}}
	// and emit {"type":"result","subtype":"success","result":"<msg>","is_error":false}
	script := "#!/bin/sh\n" +
		"while IFS= read -r line; do\n" +
		"  content=$(printf '%s' \"$line\" | sed 's/.*\"content\":\"\\([^\"]*\\)\".*/\\1/')\n" +
		"  printf '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"%s\",\"is_error\":false}\\n' \"$content\"\n" +
		"done\n"
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
	// Script emits a result whose text contains a directive line — directive
	// should be intercepted and NOT appear in content.
	script := "#!/bin/sh\n" +
		"printf '{\"type\":\"result\",\"subtype\":\"success\"," +
		"\"result\":\"{\\\\\"type\\\\\":\\\\\"spawn\\\\\",\\\\\"name\\\\\":\\\\\"w\\\\\",\\\\\"task\\\\\":\\\\\"t\\\\\"}\",\"is_error\":false}\\n'\n" +
		"cat\n"
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
