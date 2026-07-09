package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"google.golang.org/genai"
)

// buildRequest must preserve the conversation-mapping semantics the loop
// depends on: system merge, tool-call turns with thought signatures echoed,
// and consecutive tool results grouped into one user turn.
func TestBuildRequestConversationMapping(t *testing.T) {
	req := provider.CompletionRequest{
		Model:       "gemini-2.5-flash",
		Temperature: 0.7,
		MaxTokens:   1024,
		Messages: []provider.Message{
			{Role: "system", Content: "You are a test agent."},
			{Role: "user", Content: "add 2 and 3"},
			{Role: "assistant", ToolCalls: []provider.ToolCall{
				{ID: "gemini-adder-1", Name: "adder", Input: json.RawMessage(`{"a":2,"b":3}`), ThoughtSignature: "sig-abc"},
				{ID: "srv-42", Name: "checker", Input: json.RawMessage(`{}`)},
			}},
			{Role: "tool", ToolCallID: "gemini-adder-1", ToolName: "adder", Content: `{"sum":5}`},
			{Role: "tool", ToolCallID: "srv-42", ToolName: "checker", Content: `ok`, IsError: false},
			{Role: "assistant", Content: "the sum is 5"},
		},
	}
	contents, config := buildRequest(req)

	if config.SystemInstruction == nil || !strings.Contains(config.SystemInstruction.Parts[0].Text, "test agent") {
		t.Fatalf("system instruction not mapped: %+v", config.SystemInstruction)
	}
	if config.MaxOutputTokens != 1024 {
		t.Fatalf("max tokens = %d", config.MaxOutputTokens)
	}
	if config.ThinkingConfig != nil {
		t.Fatal("thinking budget must not be set at temperature 0.7")
	}
	// user, model(toolcalls), user(grouped responses), model(text)
	if len(contents) != 4 {
		t.Fatalf("contents = %d turns, want 4", len(contents))
	}
	modelTurn := contents[1]
	if modelTurn.Role != genai.RoleModel || len(modelTurn.Parts) != 2 {
		t.Fatalf("model turn wrong: %+v", modelTurn)
	}
	if string(modelTurn.Parts[0].ThoughtSignature) != "sig-abc" {
		t.Fatalf("thought signature not echoed: %q", modelTurn.Parts[0].ThoughtSignature)
	}
	if modelTurn.Parts[0].FunctionCall.ID != "" {
		t.Fatal("synthetic tool-call id must not be echoed to the API")
	}
	if modelTurn.Parts[1].FunctionCall.ID != "srv-42" {
		t.Fatal("server-assigned tool-call id must be echoed")
	}
	respTurn := contents[2]
	if len(respTurn.Parts) != 2 || respTurn.Parts[0].FunctionResponse == nil {
		t.Fatalf("tool responses not grouped into one turn: %+v", respTurn)
	}
	if respTurn.Parts[0].FunctionResponse.ID != "" {
		t.Fatal("synthetic id must not be set on functionResponse")
	}
	if respTurn.Parts[1].FunctionResponse.ID != "srv-42" {
		t.Fatal("server id must be set on functionResponse")
	}
	if out := respTurn.Parts[0].FunctionResponse.Response["output"]; out == nil {
		t.Fatalf("tool output not wrapped: %+v", respTurn.Parts[0].FunctionResponse.Response)
	}
}

// Standard JSON Schema (with $schema/additionalProperties, which Gemini's
// restricted Schema type rejects) must pass through verbatim via
// ParametersJsonSchema — the SDK-era replacement for the old sanitizer.
func TestBuildRequestToolSchemaPassthroughAndDedupe(t *testing.T) {
	schema := json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","additionalProperties":false,"properties":{"q":{"type":"string"}},"required":["q"]}`)
	req := provider.CompletionRequest{
		Temperature: 0.1,
		Messages:    []provider.Message{{Role: "user", Content: "hi"}},
		Tools: []provider.ToolDefinition{
			{Name: "search", Description: "s", InputSchema: schema},
			{Name: "search", Description: "dupe", InputSchema: schema},
			{Name: "", InputSchema: schema},
		},
	}
	_, config := buildRequest(req)
	decls := config.Tools[0].FunctionDeclarations
	if len(decls) != 1 {
		t.Fatalf("tool declarations = %d, want 1 (dedupe + drop empty name)", len(decls))
	}
	raw, _ := json.Marshal(decls[0].ParametersJsonSchema)
	for _, key := range []string{"$schema", "additionalProperties", "required"} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("schema key %q lost in passthrough: %s", key, raw)
		}
	}
	if config.ThinkingConfig == nil || *config.ThinkingConfig.ThinkingBudget != 1024 {
		t.Fatal("low-temperature requests must cap the thinking budget at 1024")
	}
}

// A tool message with unparseable content becomes a string output, and error
// results carry the retry note.
func TestFunctionResponsePart(t *testing.T) {
	p := functionResponsePart(provider.Message{Role: "tool", ToolName: "x", Content: "plain text", ToolCallID: "gemini-x-1"})
	if p.FunctionResponse.Response["output"] != "plain text" {
		t.Fatalf("plain text output mangled: %+v", p.FunctionResponse.Response)
	}
	p = functionResponsePart(provider.Message{Role: "tool", ToolName: "x", Content: `{"boom":1}`, IsError: true})
	if p.FunctionResponse.Response["error"] == nil || p.FunctionResponse.Response["note"] == nil {
		t.Fatalf("error response missing error/note: %+v", p.FunctionResponse.Response)
	}
}
