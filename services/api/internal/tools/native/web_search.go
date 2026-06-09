package native

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
			"limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
		},
		"required": []string{"query"},
	})
	return domain.Tool{
		Name:             "web_search",
		Description:      "Search the web and return a list of results with titles, URLs, and snippets",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        15000,
		Enabled:          true,
	}
}

func (t *WebSearchTool) Execute(input map[string]any) (any, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("web_search: query is required")
	}
	limit := 5
	if l, ok := input["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	apiKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	if apiKey == "" {
		return map[string]any{
			"results": []any{},
			"note":    "web search not configured — set BRAVE_SEARCH_API_KEY environment variable",
		}, nil
	}

	req, err := http.NewRequest("GET", "https://api.search.brave.com/res/v1/web/search", nil)
	if err != nil {
		return nil, fmt.Errorf("web_search: %w", err)
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("count", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("web_search: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web_search: API returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("web_search: parse response: %w", err)
	}

	results := make([]map[string]string, 0, len(raw.Web.Results))
	for _, r := range raw.Web.Results {
		results = append(results, map[string]string{
			"title":       r.Title,
			"url":         r.URL,
			"description": r.Description,
		})
	}
	return map[string]any{"results": results}, nil
}
