package directive_test

import (
	"testing"

	"nova/directive"
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
