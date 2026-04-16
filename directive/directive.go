// Package directive parses single-line JSON directives emitted by Claude agents.
package directive

import (
	"encoding/json"
	"strings"
)

// Type identifies the kind of directive.
type Type string

const (
	TypeDone    Type = "done"
	TypeRestart Type = "restart"
)

// Directive is a parsed control instruction from a Claude agent.
type Directive struct {
	Type Type `json:"type"`
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
