// Package catalogsync keeps the model catalog fresh from models.dev — an
// open-source, community-maintained model database (pricing, context windows,
// deprecations). The static provider.Catalog remains the offline fallback;
// synced data overlays it at runtime so dead or repriced models are reflected
// within a day, with no deploy.
package catalogsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
)

// sourceURL is a var so tests can point Fetch at a fixture server.
var sourceURL = "https://models.dev/api.json"

// providerAliases maps models.dev provider ids to our provider names.
var providerAliases = map[string]string{
	"anthropic": "anthropic",
	"openai":    "openai",
	"google":    "gemini",
}

// Fetch downloads and converts the models.dev catalog for the providers this
// platform supports.
func Fetch(ctx context.Context) ([]provider.ModelSpec, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned %s", res.Status)
	}

	var payload map[string]struct {
		Models map[string]struct {
			ID         string `json:"id"`
			Deprecated any    `json:"deprecated"` // null | date string | bool
			Cost       struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"cost"`
			Limit struct {
				Context int `json:"context"`
			} `json:"limit"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("models.dev decode: %w", err)
	}

	var specs []provider.ModelSpec
	for src := range providerAliases {
		entry, ok := payload[src]
		if !ok {
			continue
		}
		for id, m := range entry.Models {
			deprecated := false
			switch v := m.Deprecated.(type) {
			case bool:
				deprecated = v
			case string:
				deprecated = v != ""
			}
			specs = append(specs, provider.ModelSpec{
				Match:         strings.ToLower(id),
				ContextWindow: m.Limit.Context,
				InUSDPerMTok:  m.Cost.Input,
				OutUSDPerMTok: m.Cost.Output,
				Deprecated:    deprecated,
			})
		}
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("models.dev returned no models for supported providers")
	}
	return specs, nil
}

// Start syncs once immediately and then daily until ctx is cancelled.
// Failures log and keep the last good overlay (or the static catalog).
func Start(ctx context.Context) {
	sync := func() {
		fctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		specs, err := Fetch(fctx)
		if err != nil {
			slog.Warn("model catalog sync failed; keeping previous data", "error", err)
			return
		}
		provider.SetCatalogOverlay(specs)
		slog.Info("model catalog synced", "models", len(specs))
	}
	sync()
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sync()
			case <-ctx.Done():
				return
			}
		}
	}()
}
