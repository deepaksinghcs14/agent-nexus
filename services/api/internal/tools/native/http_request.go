package native

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

type HTTPRequestTool struct {
	client *http.Client
}

func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *HTTPRequestTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":     map[string]any{"type": "string", "description": "URL to request"},
			"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH"}},
			"headers": map[string]any{"type": "object", "description": "Optional HTTP headers"},
			"body":    map[string]any{"type": "string", "description": "Optional request body"},
		},
		"required": []string{"url"},
	})
	return domain.Tool{
		Name:             "http_request",
		Description:      "Make an HTTP request to an external URL",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        30000,
		Enabled:          true,
	}
}

func (t *HTTPRequestTool) Execute(input map[string]any) (any, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("http_request: url is required")
	}
	method, _ := input["method"].(string)
	if method == "" {
		method = "GET"
	}
	body, _ := input["body"].(string)

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http_request: build request: %w", err)
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprint(v))
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http_request: read body: %w", err)
	}

	return map[string]any{
		"status":  resp.StatusCode,
		"headers": resp.Header,
		"body":    string(respBody),
	}, nil
}
