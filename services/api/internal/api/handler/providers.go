package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/anthropic"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/gemini"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/ollama"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/openai"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type ProvidersHandler struct {
	pool      *pgxpool.Pool
	cfg       *config.Config
	providers *repository.ProviderRepository
}

func NewProvidersHandler(pool *pgxpool.Pool, cfg *config.Config) *ProvidersHandler {
	return &ProvidersHandler{pool: pool, cfg: cfg, providers: repository.NewProviderRepository(pool)}
}

func (h *ProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.providers.List(r.Context(), wsID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list providers"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *ProvidersHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	userID := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Provider    string `json:"provider"`
		DisplayName string `json:"display_name"`
		APIKey      string `json:"api_key"`
		BaseURL     string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if req.Provider == "" || req.APIKey == "" {
		errs.Write(w, errs.BadRequest("provider and api_key are required"))
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Provider
	}

	encKey, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), req.APIKey)
	if err != nil {
		errs.Write(w, errs.Internal("failed to encrypt API key"))
		return
	}

	p := &domain.ProviderCredential{
		ID:          uuid.New().String(),
		WorkspaceID: wsID,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		IsActive:    true,
		AuthType:    "api_key",
		CreatedBy:   userID,
	}

	if err := h.providers.Create(r.Context(), p, encKey); err != nil {
		errs.Write(w, errs.Internal("failed to save provider"))
		return
	}
	writeAudit(r, h.pool, "provider.created", "provider_credential", p.ID)
	errs.WriteJSON(w, http.StatusCreated, p)
}

func (h *ProvidersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, _, err := h.providers.Get(r.Context(), id)
	if err != nil {
		errs.Write(w, errs.NotFound("provider not found"))
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
		APIKey      string `json:"api_key"`
		BaseURL     string `json:"base_url"`
		IsActive    *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	encKey := ""
	if req.APIKey != "" {
		encKey, err = encrypt.Encrypt([]byte(h.cfg.EncryptionKey), req.APIKey)
		if err != nil {
			errs.Write(w, errs.Internal("failed to encrypt API key"))
			return
		}
	}
	if req.DisplayName != "" {
		existing.DisplayName = req.DisplayName
	}
	if req.BaseURL != "" {
		existing.BaseURL = req.BaseURL
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := h.providers.Update(r.Context(), existing, encKey); err != nil {
		errs.Write(w, errs.Internal("failed to update provider"))
		return
	}
	writeAudit(r, h.pool, "provider.updated", "provider_credential", existing.ID)
	errs.WriteJSON(w, http.StatusOK, existing)
}

func (h *ProvidersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	if err := h.providers.Delete(r.Context(), providerID); err != nil {
		errs.Write(w, errs.Internal("failed to delete provider"))
		return
	}
	writeAudit(r, h.pool, "provider.deleted", "provider_credential", providerID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProvidersHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cred, encKey, err := h.providers.Get(r.Context(), id)
	if err != nil {
		errs.Write(w, errs.NotFound("provider not found"))
		return
	}

	adapter, adapterErr := h.adapterFor(r.Context(), cred, encKey)
	if adapterErr != nil {
		slog.Warn("failed to build provider adapter for model listing", "error", adapterErr)
		errs.WriteJSON(w, http.StatusOK, map[string]any{"data": staticModels(cred.Provider)})
		return
	}

	models, err := adapter.Models(r.Context())
	if err != nil {
		slog.Warn("live model fetch failed, using static fallback", "provider", cred.Provider, "error", err)
		errs.WriteJSON(w, http.StatusOK, map[string]any{"data": staticModels(cred.Provider)})
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": models})
}

// adapterFor instantiates the correct provider adapter for a credential.
// Handles both API-key and OAuth credentials.
func (h *ProvidersHandler) adapterFor(ctx context.Context, cred *domain.ProviderCredential, encKey string) (provider.Provider, error) {
	if cred.AuthType == "oauth" && cred.Provider == "gemini" {
		if h.cfg.GoogleOAuthClientID == "" {
			return nil, fmt.Errorf("google oauth not configured")
		}
		accessToken, err := h.providers.GetDecryptedAccessToken(ctx, cred.ID, h.cfg.EncryptionKey, h.cfg.GoogleOAuthClientID, h.cfg.GoogleOAuthClientSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth access token: %w", err)
		}
		return gemini.New(accessToken, "oauth"), nil
	}

	if encKey == "" {
		switch cred.Provider {
		case "ollama":
			return ollama.New(cred.BaseURL), nil
		default:
			return nil, fmt.Errorf("no api key configured for %s", cred.Provider)
		}
	}
	apiKey, err := encrypt.Decrypt([]byte(h.cfg.EncryptionKey), encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt api key: %w", err)
	}
	switch cred.Provider {
	case "anthropic":
		return anthropic.New(apiKey, cred.BaseURL), nil
	case "openai":
		return openai.New(apiKey, cred.BaseURL), nil
	case "gemini":
		return gemini.New(apiKey, "api_key"), nil
	case "ollama":
		return ollama.New(cred.BaseURL), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cred.Provider)
	}
}

// OAuthGoogleAuthorize generates the Google OAuth URL and returns it as JSON.
// The frontend calls this via fetch (with Authorization header), then redirects the browser.
func (h *ProvidersHandler) OAuthGoogleAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.cfg.GoogleOAuthClientID == "" {
		errs.Write(w, errs.BadRequest("google oauth is not configured on this server"))
		return
	}
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	userID := middleware.UserIDFromCtx(r.Context())

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		errs.Write(w, errs.Internal("failed to generate state"))
		return
	}
	state := hex.EncodeToString(stateBytes)

	if err := h.providers.CreateOAuthState(r.Context(), state, wsID, userID); err != nil {
		errs.Write(w, errs.Internal("failed to store oauth state"))
		return
	}

	redirectURI := strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/api/v1/providers/oauth/google/callback"

	params := url.Values{
		"client_id":     {h.cfg.GoogleOAuthClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"https://www.googleapis.com/auth/generative-language"},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
	errs.WriteJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

// OAuthGoogleCallback receives the authorization code from Google and exchanges it for tokens.
func (h *ProvidersHandler) OAuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	errParam := r.URL.Query().Get("error")

	frontendURL := strings.Join(h.cfg.CORSOrigins, "")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	if errParam != "" {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason="+url.QueryEscape(errParam), http.StatusFound)
		return
	}
	if state == "" || code == "" {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=missing_params", http.StatusFound)
		return
	}

	workspaceID, userID, err := h.providers.ConsumeOAuthState(r.Context(), state)
	if err != nil {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=invalid_state", http.StatusFound)
		return
	}

	redirectURI := strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/api/v1/providers/oauth/google/callback"

	// Exchange code for tokens
	vals := url.Values{
		"client_id":     {h.cfg.GoogleOAuthClientID},
		"client_secret": {h.cfg.GoogleOAuthClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(vals.Encode()))
	if err != nil {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=token_request_failed", http.StatusFound)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=token_exchange_failed", http.StatusFound)
		return
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.AccessToken == "" {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=token_parse_failed", http.StatusFound)
		return
	}

	encAccess, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), tok.AccessToken)
	if err != nil {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=encrypt_failed", http.StatusFound)
		return
	}
	encRefresh, err := encrypt.Encrypt([]byte(h.cfg.EncryptionKey), tok.RefreshToken)
	if err != nil {
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=encrypt_failed", http.StatusFound)
		return
	}

	expiry := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	scopes := strings.Fields(tok.Scope)

	if err := h.providers.UpsertOAuthCredential(r.Context(), workspaceID, "gemini", "Google Gemini (OAuth)", encAccess, encRefresh, expiry, scopes, userID); err != nil {
		slog.Error("failed to store oauth credential", "error", err)
		http.Redirect(w, r, frontendURL+"/settings/providers?oauth=error&reason=store_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, frontendURL+"/settings/providers?oauth=success", http.StatusFound)
}

func staticModels(providerName string) []map[string]any {
	switch providerName {
	case "anthropic":
		return []map[string]any{
			{"id": "claude-opus-4-8", "name": "Claude Opus 4.8", "context_window": 200000, "supports_tools": true},
			{"id": "claude-sonnet-4-6", "name": "Claude Sonnet 4.6", "context_window": 200000, "supports_tools": true},
			{"id": "claude-haiku-4-5-20251001", "name": "Claude Haiku 4.5", "context_window": 200000, "supports_tools": true},
		}
	case "openai":
		return []map[string]any{
			{"id": "gpt-4o", "name": "GPT-4o", "context_window": 128000, "supports_tools": true},
			{"id": "gpt-4o-mini", "name": "GPT-4o Mini", "context_window": 128000, "supports_tools": true},
			{"id": "o1", "name": "o1", "context_window": 200000, "supports_tools": false},
		}
	case "gemini":
		return []map[string]any{
			{"id": "gemini-2.5-pro", "name": "Gemini 2.5 Pro", "context_window": 1000000, "supports_tools": true},
			{"id": "gemini-2.5-flash", "name": "Gemini 2.5 Flash", "context_window": 1000000, "supports_tools": true},
		}
	case "ollama":
		return []map[string]any{}
	}
	return []map[string]any{}
}
