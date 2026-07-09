// Package sse builds the JSON event lines streamed to clients over
// Server-Sent Events. Every event the API emits is constructed here, so the
// wire protocol lives in one place instead of ~100 hand-written fmt.Sprintf
// JSON literals scattered through the handler package. Constructors return
// the marshalled string because the existing run loops pass events around as
// plain strings (emit func(string)).
package sse

import "encoding/json"

// evt marshals a type tag plus fields. Marshalling a map keeps constructors
// terse; the per-event constructors below are the typed surface callers use.
func evt(typ string, fields map[string]any) string {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["type"] = typ
	b, err := json.Marshal(fields)
	if err != nil {
		// Fields are built from JSON-safe values in this package; a failure
		// here is a programming error. Emit a well-formed fallback so the
		// stream never carries invalid JSON.
		fb, _ := json.Marshal(map[string]string{"type": "error", "error": "internal: event marshal failed"})
		return string(fb)
	}
	return string(b)
}

// rawOrString returns b as raw JSON when it already parses, else as a JSON
// string — the same tolerance the old jsonOrStr helpers gave tool inputs and
// outputs that may or may not be valid JSON.
func rawOrString(b []byte) any {
	if json.Valid(b) {
		return json.RawMessage(b)
	}
	return string(b)
}

func Ping() string { return `{"type":"ping"}` }

func Delta(content string) string {
	return evt("delta", map[string]any{"content": content})
}

func DeltaForNode(nodeID, nodeName, content string) string {
	return evt("delta", map[string]any{"content": content, "node_id": nodeID, "node_name": nodeName})
}

func Error(msg string) string {
	return evt("error", map[string]any{"error": msg})
}

func ErrorForNode(nodeID, msg string) string {
	return evt("error", map[string]any{"node_id": nodeID, "error": msg})
}

func RunStarted(runID string) string {
	return evt("run_started", map[string]any{"run_id": runID})
}

func RunCompleted(runID string, inputTokens, outputTokens int, cost float64) string {
	return evt("run_completed", map[string]any{
		"run_id": runID,
		"usage":  map[string]int{"input": inputTokens, "output": outputTokens},
		"cost":   cost,
	})
}

func ToolStarted(callID, tool string, input []byte) string {
	return evt("tool_started", map[string]any{"call_id": callID, "tool": tool, "input": rawOrString(input)})
}

func ToolCall(callID, tool string, input, output []byte, latencyMs int) string {
	return evt("tool_call", map[string]any{
		"call_id": callID, "tool": tool,
		"input": rawOrString(input), "output": rawOrString(output),
		"latency_ms": latencyMs,
	})
}

func ApprovalRequired(tool string, input []byte, approvalID string) string {
	return evt("approval_required", map[string]any{"tool": tool, "input": rawOrString(input), "approval_id": approvalID})
}

func ApprovalParked(runID, approvalID string) string {
	return evt("approval_parked", map[string]any{"run_id": runID, "approval_id": approvalID})
}

func SessionParked(runID string) string {
	return evt("session_parked", map[string]any{"run_id": runID})
}

func NodeStarted(nodeID, nodeType, nodeName string) string {
	return evt("node_started", map[string]any{"node_id": nodeID, "node_type": nodeType, "node_name": nodeName})
}

func NodeCompleted(nodeID, nodeName string) string {
	return evt("node_completed", map[string]any{"node_id": nodeID, "node_name": nodeName})
}

func NodeResumed(nodeID, nodeName string) string {
	return evt("node_resumed", map[string]any{"node_id": nodeID, "node_name": nodeName})
}

func NodeRouted(nodeID, result, nextNodeID string) string {
	return evt("node_routed", map[string]any{"node_id": nodeID, "result": result, "next_node_id": nextNodeID})
}

func NodeRoutedLoop(nodeID, result string, iteration, max int) string {
	return evt("node_routed", map[string]any{"node_id": nodeID, "result": result, "iteration": iteration, "max": max})
}

func NodeDelivery(nodeID, target string, err error) string {
	f := map[string]any{"node_id": nodeID, "target": target, "ok": err == nil}
	if err != nil {
		f["error"] = err.Error()
	}
	return evt("node_delivery", f)
}
