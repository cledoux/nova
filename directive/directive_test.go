package directive_test

import (
	"testing"

	"nova/directive"
)

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

func TestParse_restart(t *testing.T) {
	d, err := directive.Parse(`{"type":"restart"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d == nil {
		t.Fatal("expected directive")
	}
	if d.Type != directive.TypeRestart {
		t.Errorf("Type = %q, want restart", d.Type)
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
	_, err := directive.Parse(`{"type":"restart"`)
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
