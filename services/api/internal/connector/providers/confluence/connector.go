package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// httpClient with a per-request timeout so hung connections on Railway don't stall the sync.
var httpClient = &http.Client{Timeout: 30 * time.Second}

type Connector struct{}

func New() *Connector { return &Connector{} }

// checkpoint is the resume state persisted after each page batch.
type checkpoint struct {
	CompletedSpaces []string `json:"completed_spaces"`
	CurrentSpace    string   `json:"current_space"`
	CurrentOffset   int      `json:"current_offset"`
}

// FetchStream implements connector.StreamProvider.
// It emits pages one batch at a time and checkpoints after each batch so a pod restart
// can resume from the last successfully persisted page offset.
func (c *Connector) FetchStream(ctx context.Context, cfg map[string]any, opts connector.FetchOpts, emit func(connector.Document) error) error {
	baseURL, _ := cfg["url"].(string)
	username, _ := cfg["username"].(string)
	token, _ := cfg["token"].(string)
	spaceKeys, _ := cfg["space_keys"].(string)

	if baseURL == "" || username == "" || token == "" {
		return fmt.Errorf("confluence: url, username, and token are required")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	log := slog.With("connector", "confluence", "base_url", baseURL)

	do := func(rawURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, token)
		req.Header.Set("Accept", "application/json")
		return httpClient.Do(req)
	}

	// Load resume state from checkpoint.
	var cp checkpoint
	if len(opts.Checkpoint) > 2 {
		_ = json.Unmarshal(opts.Checkpoint, &cp)
	}
	completedSet := make(map[string]bool, len(cp.CompletedSpaces))
	for _, s := range cp.CompletedSpaces {
		completedSet[s] = true
	}

	// Resolve which spaces to sync, collecting names for metadata.
	type spaceInfo struct{ key, name string }
	var spaceList []spaceInfo
	spaceNames := map[string]string{}

	if spaceKeys != "" {
		for _, k := range strings.Split(spaceKeys, ",") {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			// Fetch the space name so it can be stored in document metadata.
			name := k
			res, err := do(fmt.Sprintf("%s/wiki/rest/api/space/%s", baseURL, url.PathEscape(k)))
			if err == nil && res.StatusCode == http.StatusOK {
				var s struct{ Name string `json:"name"` }
				_ = json.NewDecoder(res.Body).Decode(&s)
				res.Body.Close()
				if s.Name != "" {
					name = s.Name
				}
			} else if res != nil {
				res.Body.Close()
			}
			spaceList = append(spaceList, spaceInfo{k, name})
			spaceNames[k] = name
		}
	} else {
		start := 0
		for {
			u := fmt.Sprintf("%s/wiki/rest/api/space?limit=50&start=%d&type=global", baseURL, start)
			res, err := do(u)
			if err != nil {
				return fmt.Errorf("confluence: list spaces: %w", err)
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				return fmt.Errorf("confluence: list spaces returned HTTP %d (check credentials)", res.StatusCode)
			}
			var page struct {
				Results []struct {
					Key  string `json:"key"`
					Name string `json:"name"`
				} `json:"results"`
				Size  int `json:"size"`
				Limit int `json:"limit"`
			}
			_ = json.NewDecoder(res.Body).Decode(&page)
			res.Body.Close()
			for _, s := range page.Results {
				spaceList = append(spaceList, spaceInfo{s.Key, s.Name})
				spaceNames[s.Key] = s.Name
			}
			log.Debug("discovered spaces", "page_start", start, "page_count", len(page.Results))
			if page.Size < page.Limit {
				break
			}
			start += page.Limit
		}
	}

	// Build legacy spaces []string for checkpoint compatibility.
	spaces := make([]string, len(spaceList))
	for i, s := range spaceList {
		spaces[i] = s.key
	}

	log.Info("syncing spaces", "total", len(spaces), "already_completed", len(completedSet))

	for _, spaceKey := range spaces {
		if completedSet[spaceKey] {
			log.Info("skipping already-completed space", "space", spaceKey)
			continue
		}

		// Resume this space from the last checkpoint offset if it was interrupted mid-space.
		start := 0
		if spaceKey == cp.CurrentSpace && cp.CurrentOffset > 0 {
			start = cp.CurrentOffset
			log.Info("resuming space from checkpoint", "space", spaceKey, "offset", start)
		}

		spaceTotal := 0
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var u string
			if !opts.LastSyncedAt.IsZero() {
				// CQL search instead of the plain listing endpoint: server-side
				// filters to pages actually modified since the last successful
				// sync, so an unchanged space costs one near-empty page instead
				// of paginating through everything just to re-hash it.
				cql := fmt.Sprintf(`type=page and space="%s" and lastModified>"%s"`,
					spaceKey, opts.LastSyncedAt.UTC().Format("2006-01-02 15:04"))
				u = fmt.Sprintf(
					"%s/wiki/rest/api/content/search?cql=%s&expand=body.storage,history.createdBy&limit=50&start=%d",
					baseURL, url.QueryEscape(cql), start,
				)
			} else {
				u = fmt.Sprintf(
					"%s/wiki/rest/api/content?type=page&spaceKey=%s&expand=body.storage,history.createdBy&limit=50&start=%d",
					baseURL, url.QueryEscape(spaceKey), start,
				)
			}
			res, err := do(u)
			if err != nil {
				log.Warn("fetch pages failed", "space", spaceKey, "offset", start, "error", err)
				break
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				log.Warn("fetch pages non-200", "space", spaceKey, "status", res.StatusCode)
				break
			}
			var page struct {
				Results []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					Body  struct {
						Storage struct{ Value string `json:"value"` } `json:"storage"`
					} `json:"body"`
					History struct {
						CreatedBy struct{ DisplayName string `json:"displayName"` } `json:"createdBy"`
					} `json:"history"`
					Links struct{ WebUI string `json:"webui"` } `json:"_links"`
				} `json:"results"`
				Size  int `json:"size"`
				Limit int `json:"limit"`
			}
			_ = json.NewDecoder(res.Body).Decode(&page)
			res.Body.Close()

			log.Debug("processing page batch", "space", spaceKey, "offset", start, "batch_size", len(page.Results))

			for _, r := range page.Results {
				text := html.UnescapeString(htmlTagRe.ReplaceAllString(r.Body.Storage.Value, " "))
				text = strings.Join(strings.Fields(text), " ")
				if err := emit(connector.Document{
					Source:           "confluence",
					SourceDocumentID: r.ID,
					Title:            r.Title,
					URL:              baseURL + "/wiki" + r.Links.WebUI,
					Author:           r.History.CreatedBy.DisplayName,
					Content:          text,
					Metadata:         map[string]any{"space_key": spaceKey, "space_name": spaceNames[spaceKey]},
				}); err != nil {
					return err // context cancelled or caller abort
				}
			}
			spaceTotal += len(page.Results)

			// Checkpoint after each page batch so a pod restart can resume mid-space.
			if opts.SaveCheckpoint != nil {
				opts.SaveCheckpoint(checkpoint{
					CompletedSpaces: cp.CompletedSpaces,
					CurrentSpace:    spaceKey,
					CurrentOffset:   start + len(page.Results),
				})
			}

			if page.Size < page.Limit {
				break
			}
			start += page.Limit
		}

		log.Info("space synced", "space", spaceKey, "pages", spaceTotal)

		// Mark this space as fully completed and clear the in-progress offset.
		cp.CompletedSpaces = append(cp.CompletedSpaces, spaceKey)
		completedSet[spaceKey] = true
		if opts.SaveCheckpoint != nil {
			opts.SaveCheckpoint(checkpoint{
				CompletedSpaces: cp.CompletedSpaces,
				CurrentSpace:    "",
				CurrentOffset:   0,
			})
		}
	}

	log.Info("all spaces synced", "total_spaces", len(spaces))
	return nil
}

// Fetch implements connector.Provider via FetchStream for callers that don't use streaming.
func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	var docs []connector.Document
	err := c.FetchStream(ctx, cfg, connector.FetchOpts{}, func(doc connector.Document) error {
		docs = append(docs, doc)
		return nil
	})
	return docs, err
}
