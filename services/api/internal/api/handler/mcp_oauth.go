package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/mcp"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// mcpOAuthDoc is the per-server OAuth state, stored AES-256-GCM-encrypted in
// mcp_servers.oauth. It holds the dynamic client registration, the discovered
// endpoints, the live tokens, and (transiently) the pending PKCE flow.
type mcpOAuthDoc struct {
	ClientID              string    `json:"client_id"`
	ClientSecret          string    `json:"client_secret,omitempty"`
	AuthorizationEndpoint string    `json:"authorization_endpoint"`
	TokenEndpoint         string    `json:"token_endpoint"`
	RegistrationEndpoint  string    `json:"registration_endpoint,omitempty"`
	Resource              string    `json:"resource,omitempty"`
	Scope                 string    `json:"scope,omitempty"`
	AccessToken           string    `json:"access_token,omitempty"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	ExpiresAt             time.Time `json:"expires_at,omitempty"`
	PendingState          string    `json:"pending_state,omitempty"`
	PendingVerifier       string    `json:"pending_verifier,omitempty"`
	PendingRedirect       string    `json:"pending_redirect,omitempty"`
}

func loadMCPOAuthDoc(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, serverID string) (*mcpOAuthDoc, error) {
	var enc *string
	if err := pool.QueryRow(ctx, `SELECT oauth FROM mcp_servers WHERE id=$1::uuid`, serverID).Scan(&enc); err != nil {
		return nil, fmt.Errorf("mcp server not found: %w", err)
	}
	if enc == nil || *enc == "" {
		return &mcpOAuthDoc{}, nil
	}
	plain, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), *enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt oauth state: %w", err)
	}
	var doc mcpOAuthDoc
	if err := json.Unmarshal([]byte(plain), &doc); err != nil {
		return nil, fmt.Errorf("parse oauth state: %w", err)
	}
	return &doc, nil
}

func saveMCPOAuthDoc(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, serverID string, doc *mcpOAuthDoc) error {
	plain, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	enc, err := encrypt.Encrypt([]byte(cfg.EncryptionKey), string(plain))
	if err != nil {
		return fmt.Errorf("encrypt oauth state: %w", err)
	}
	_, err = pool.Exec(ctx,
		`UPDATE mcp_servers SET oauth=$2, auth_type='oauth', updated_at=NOW() WHERE id=$1::uuid`,
		serverID, enc)
	return err
}

// OAuthStart handles POST /mcp-servers/{id}/oauth/start. It discovers the
// server's authorization server, registers a client if needed, and returns
// the browser authorization URL the user must open to grant access.
func (h *MCPHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	s, err := scanMCP(h.pool.QueryRow(r.Context(), mcpSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), ws))
	if err != nil {
		errs.Write(w, errs.NotFound("MCP server not found"))
		return
	}
	if s.Transport != "http" {
		errs.Write(w, errs.BadRequest("OAuth is only supported for http-transport MCP servers"))
		return
	}

	meta, resource, err := mcp.DiscoverAuthServer(r.Context(), s.URL)
	if err != nil {
		errs.Write(w, errs.BadRequest("authorization discovery failed: "+err.Error()))
		return
	}

	doc, err := loadMCPOAuthDoc(r.Context(), h.pool, h.cfg, s.ID)
	if err != nil {
		doc = &mcpOAuthDoc{} // corrupt/unreadable prior state — start fresh
	}
	redirectURI := strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/api/v1/mcp-servers/oauth/callback"

	// (Re)register when we have no client yet or the auth server moved.
	if doc.ClientID == "" || doc.TokenEndpoint != meta.TokenEndpoint {
		if meta.RegistrationEndpoint == "" {
			errs.Write(w, errs.BadRequest("authorization server does not support dynamic client registration"))
			return
		}
		clientID, clientSecret, err := mcp.RegisterClient(r.Context(), meta.RegistrationEndpoint, redirectURI, "Agent Nexus")
		if err != nil {
			errs.Write(w, errs.BadRequest(err.Error()))
			return
		}
		doc.ClientID, doc.ClientSecret = clientID, clientSecret
	}

	doc.AuthorizationEndpoint = meta.AuthorizationEndpoint
	doc.TokenEndpoint = meta.TokenEndpoint
	doc.RegistrationEndpoint = meta.RegistrationEndpoint
	doc.Resource = resource
	doc.Scope = strings.Join(meta.ScopesSupported, " ")

	verifier, challenge := mcp.NewPKCE()
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	doc.PendingState = hex.EncodeToString(nonce)
	doc.PendingVerifier = verifier
	doc.PendingRedirect = redirectURI
	if err := saveMCPOAuthDoc(r.Context(), h.pool, h.cfg, s.ID, doc); err != nil {
		errs.Write(w, errs.Internal("failed to persist oauth state: "+err.Error()))
		return
	}

	// The server ID rides in the state so the (unauthenticated) callback can
	// locate the right row; the random nonce is what actually gets verified.
	state := s.ID + "." + doc.PendingState
	authorizeURL := mcp.AuthorizeURL(meta, doc.ClientID, redirectURI, state, challenge, resource, doc.Scope)
	writeAudit(r, h.pool, "mcp_server.oauth_started", "mcp_server", s.ID)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"authorize_url": authorizeURL})
}

// OAuthCallback handles GET /mcp-servers/oauth/callback — the browser
// redirect from the authorization server. Unauthenticated by nature; the
// state nonce persisted by OAuthStart is the proof of a legitimate flow.
func (h *MCPHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	renderErr := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "<html><body><h3>Authorization failed</h3><p>%s</p></body></html>", html.EscapeString(msg))
	}
	if e := q.Get("error"); e != "" {
		renderErr(e + ": " + q.Get("error_description"))
		return
	}
	code, state := q.Get("code"), q.Get("state")
	serverID, nonce, ok := strings.Cut(state, ".")
	if code == "" || !ok || serverID == "" || nonce == "" {
		renderErr("missing or malformed code/state")
		return
	}

	doc, err := loadMCPOAuthDoc(r.Context(), h.pool, h.cfg, serverID)
	if err != nil {
		renderErr("unknown server")
		return
	}
	if doc.PendingState == "" || subtle.ConstantTimeCompare([]byte(doc.PendingState), []byte(nonce)) != 1 {
		renderErr("state mismatch — restart the authorization from Agent Nexus")
		return
	}

	token, err := mcp.ExchangeCode(r.Context(), doc.TokenEndpoint, doc.ClientID, doc.ClientSecret,
		code, doc.PendingRedirect, doc.PendingVerifier, doc.Resource)
	if err != nil {
		renderErr(err.Error())
		return
	}
	applyToken(doc, token)
	doc.PendingState, doc.PendingVerifier, doc.PendingRedirect = "", "", ""
	if err := saveMCPOAuthDoc(r.Context(), h.pool, h.cfg, serverID, doc); err != nil {
		renderErr("failed to persist tokens")
		return
	}
	h.pool.Exec(r.Context(), `UPDATE mcp_servers SET status='connected', updated_at=NOW() WHERE id=$1::uuid`, serverID) //nolint:errcheck

	// Sync tools in the background so the server is usable immediately.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		s, err := scanMCP(h.pool.QueryRow(ctx, mcpSelect+` WHERE id=$1::uuid`, serverID))
		if err != nil {
			return
		}
		if n, err := h.syncServerTools(ctx, s); err != nil {
			slog.Warn("post-oauth tool sync failed", "server_id", serverID, "error", err)
		} else {
			slog.Info("post-oauth tool sync complete", "server_id", serverID, "tools", n)
		}
	}()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<html><body><h3>Authorization complete</h3><p>Agent Nexus is now connected. You can close this window.</p></body></html>")
}

// ── token manager & client factory ───────────────────────────────────────────

var mcpOAuthLocks sync.Map // serverID → *sync.Mutex

// mcpOAuthAccessToken returns a valid access token for an oauth MCP server,
// refreshing (and persisting) it when expired. Serialized per server so
// concurrent tool calls don't race the refresh grant.
func mcpOAuthAccessToken(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, serverID string) (string, error) {
	muAny, _ := mcpOAuthLocks.LoadOrStore(serverID, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	doc, err := loadMCPOAuthDoc(ctx, pool, cfg, serverID)
	if err != nil {
		return "", err
	}
	if doc.AccessToken != "" && time.Until(doc.ExpiresAt) > 60*time.Second {
		return doc.AccessToken, nil
	}
	if doc.RefreshToken == "" {
		return "", fmt.Errorf("mcp server requires (re-)authorization — run the OAuth flow")
	}
	token, err := mcp.RefreshToken(ctx, doc.TokenEndpoint, doc.ClientID, doc.ClientSecret, doc.RefreshToken, doc.Resource)
	if err != nil {
		return "", fmt.Errorf("token refresh failed (re-authorize the MCP server): %w", err)
	}
	applyToken(doc, token)
	if err := saveMCPOAuthDoc(ctx, pool, cfg, serverID, doc); err != nil {
		return "", err
	}
	return doc.AccessToken, nil
}

func applyToken(doc *mcpOAuthDoc, t *mcp.Token) {
	doc.AccessToken = t.AccessToken
	if t.RefreshToken != "" { // rotation: keep the old one unless replaced
		doc.RefreshToken = t.RefreshToken
	}
	expiresIn := t.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	doc.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

// mcpClientForServer builds an MCP client for a server row, resolving a fresh
// OAuth access token when the server uses auth_type='oauth'. Static-config
// servers keep the existing behavior (token parsed from config JSON).
func mcpClientForServer(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, id, url, transport, authType string, config json.RawMessage) (*mcp.Client, error) {
	if authType == "oauth" {
		tok, err := mcpOAuthAccessToken(ctx, pool, cfg, id)
		if err != nil {
			return nil, err
		}
		c, _ := json.Marshal(map[string]string{"token": tok})
		return mcp.NewClient(id, url, transport, c), nil
	}
	return mcp.NewClient(id, url, transport, config), nil
}
