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
	TypeRestart       Type = "restart"
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
