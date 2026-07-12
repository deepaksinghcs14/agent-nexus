package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// HTTPToolConfig is stored in the tool's Config JSONB column.
type HTTPToolConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	QueryParams  map[string]string `json:"query_params"`
	BodyMode     string            `json:"body_mode"`     // "template" | "free"
	BodyTemplate string            `json:"body_template"` // used when body_mode == "template"
}

var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// substituteVars replaces {{varName}} tokens in s with the matching value from vars.
// Values that are not strings are JSON-serialised (without quotes for numbers/bools).
func substituteVars(s string, vars map[string]any) string {
	return templateVarRe.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-2]
		v, ok := vars[key]
		if !ok {
			return match
		}
		switch val := v.(type) {
		case string:
			return val
		default:
			b, _ := json.Marshal(val)
			return string(b)
		}
	})
}

// ExecuteHTTP runs an HTTP tool call using the stored config and the LLM-provided input.
func ExecuteHTTP(ctx context.Context, cfg HTTPToolConfig, rawInput json.RawMessage, timeoutMs int) *ExecutionResult {
	start := time.Now()

	var input map[string]any
	_ = json.Unmarshal(rawInput, &input)
	if input == nil {
		input = map[string]any{}
	}

	// Substitute vars in URL
	resolvedURL := substituteVars(cfg.URL, input)

	// Build query string
	if len(cfg.QueryParams) > 0 {
		params := url.Values{}
		for k, v := range cfg.QueryParams {
			params.Set(k, substituteVars(v, input))
		}
		sep := "?"
		if strings.Contains(resolvedURL, "?") {
			sep = "&"
		}
		resolvedURL += sep + params.Encode()
	}

	// Build body
	var bodyReader io.Reader
	switch cfg.BodyMode {
	case "template":
		if cfg.BodyTemplate != "" {
			bodyReader = strings.NewReader(substituteVars(cfg.BodyTemplate, input))
		}
	default: // "free" or unset — LLM provides body directly
		if body, ok := input["body"]; ok {
			b, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(b)
		} else {
			// whole input is the body
			b, _ := json.Marshal(input)
			bodyReader = bytes.NewReader(b)
		}
	}

	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = "POST"
	}
	if method == "GET" || method == "DELETE" || method == "HEAD" {
		bodyReader = nil
	}

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, method, resolvedURL, bodyReader)
	if err != nil {
		return &ExecutionResult{LatencyMs: int(time.Since(start).Milliseconds()), Error: fmt.Sprintf("build request: %s", err)}
	}

	// Default Content-Type for requests with a body
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply configured headers (with var substitution, overrides default Content-Type)
	for k, v := range cfg.Headers {
		req.Header.Set(k, substituteVars(v, input))
	}

	resp, err := client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return &ExecutionResult{LatencyMs: latencyMs, Error: fmt.Sprintf("request failed: %s", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap

	// Try to parse response as JSON, fall back to plain string
	var output any
	if err := json.Unmarshal(respBody, &output); err != nil {
		output = string(respBody)
	}

	result := &ExecutionResult{Output: output, LatencyMs: latencyMs}
	if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return result
}
