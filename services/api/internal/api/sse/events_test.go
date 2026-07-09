package sse

import (
	"encoding/json"
	"testing"
)

// Every constructor must produce valid JSON with the right type tag —
// the property the old hand-built fmt.Sprintf literals could silently break.
func TestEventsAreValidJSON(t *testing.T) {
	cases := map[string]string{
		"ping":                Ping(),
		"delta":               Delta("hello \"quoted\" text\nwith newline"),
		"error":               Error(`msg with "quotes" and \ backslash`),
		"run_completed":       RunCompleted("r1", 10, 20, 0.005),
		"tool_started":        ToolStarted("c1", "search", []byte(`{"q":"x"}`)),
		"tool_call":           ToolCall("c1", "search", []byte(`not-json`), []byte(`{"ok":true}`), 42),
		"approval_required":   ApprovalRequired("deploy", []byte(`{"env":"prod"}`), "a1"),
		"node_started":        NodeStarted("n1", "agent", "Writer"),
		"node_delivery":       NodeDelivery("n1", "webhook", nil),
		"user_input_required": UserInputRequired("r1", "which env?"),
	}
	for name, raw := range cases {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Errorf("%s: invalid JSON %q: %v", name, raw, err)
			continue
		}
		if m["type"] == "" || m["type"] == nil {
			t.Errorf("%s: missing type tag in %q", name, raw)
		}
	}
}

// Tool inputs that aren't valid JSON must be embedded as JSON strings, and
// valid JSON must pass through raw — matching the old jsonOrStr behaviour.
func TestToolCallRawVsString(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal([]byte(ToolCall("c", "t", []byte(`{"a":1}`), []byte(`plain`), 0)), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["input"].(map[string]any); !ok {
		t.Errorf("valid JSON input should stay raw, got %T", m["input"])
	}
	if _, ok := m["output"].(string); !ok {
		t.Errorf("non-JSON output should become a string, got %T", m["output"])
	}
}
