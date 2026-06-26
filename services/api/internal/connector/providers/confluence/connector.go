package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	baseURL, _ := cfg["url"].(string)
	username, _ := cfg["username"].(string)
	token, _ := cfg["token"].(string)
	spaceKeys, _ := cfg["space_keys"].(string)

	if baseURL == "" || username == "" || token == "" {
		return nil, fmt.Errorf("confluence connector: url, username, and token are required")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	do := func(rawURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, token)
		req.Header.Set("Accept", "application/json")
		return http.DefaultClient.Do(req)
	}

	// Resolve space keys to fetch
	var spaces []string
	if spaceKeys != "" {
		for _, k := range strings.Split(spaceKeys, ",") {
			if k = strings.TrimSpace(k); k != "" {
				spaces = append(spaces, k)
			}
		}
	} else {
		// List all spaces
		start := 0
		for {
			u := fmt.Sprintf("%s/wiki/rest/api/space?limit=50&start=%d", baseURL, start)
			res, err := do(u)
			if err != nil {
				return nil, fmt.Errorf("confluence connector: failed to list spaces: %w", err)
			}
			var page struct {
				Results []struct {
					Key string `json:"key"`
				} `json:"results"`
				Size  int `json:"size"`
				Limit int `json:"limit"`
			}
			err = json.NewDecoder(res.Body).Decode(&page)
			res.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("confluence connector: failed to decode spaces: %w", err)
			}
			for _, s := range page.Results {
				spaces = append(spaces, s.Key)
			}
			if page.Size < page.Limit {
				break
			}
			start += page.Limit
		}
	}

	var docs []connector.Document
	for _, spaceKey := range spaces {
		start := 0
		for {
			u := fmt.Sprintf(
				"%s/wiki/rest/api/content?type=page&spaceKey=%s&expand=body.storage,history.createdBy&limit=50&start=%d",
				baseURL, url.QueryEscape(spaceKey), start,
			)
			res, err := do(u)
			if err != nil {
				break
			}
			var page struct {
				Results []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					Body  struct {
						Storage struct {
							Value string `json:"value"`
						} `json:"storage"`
					} `json:"body"`
					History struct {
						CreatedBy struct {
							DisplayName string `json:"displayName"`
						} `json:"createdBy"`
					} `json:"history"`
					Links struct {
						WebUI string `json:"webui"`
					} `json:"_links"`
				} `json:"results"`
				Size  int `json:"size"`
				Limit int `json:"limit"`
			}
			err = json.NewDecoder(res.Body).Decode(&page)
			res.Body.Close()
			if err != nil {
				break
			}

			for _, r := range page.Results {
				text := html.UnescapeString(htmlTagRe.ReplaceAllString(r.Body.Storage.Value, " "))
				// Collapse whitespace
				text = strings.Join(strings.Fields(text), " ")
				docs = append(docs, connector.Document{
					Source:           "confluence",
					SourceDocumentID: r.ID,
					Title:            r.Title,
					URL:              baseURL + "/wiki" + r.Links.WebUI,
					Author:           r.History.CreatedBy.DisplayName,
					Content:          text,
				})
			}

			if page.Size < page.Limit {
				break
			}
			start += page.Limit
		}
	}

	return docs, nil
}
