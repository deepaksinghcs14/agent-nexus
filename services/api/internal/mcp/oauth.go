package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth 2.1 client support for remote MCP servers, per the MCP authorization
// spec: protected-resource discovery (RFC 9728) → authorization-server
// metadata (RFC 8414 / OIDC discovery) → dynamic client registration
// (RFC 7591) → authorization-code grant with PKCE (RFC 7636) and resource
// indicators (RFC 8707) → refresh grant.

// ASMetadata is the subset of authorization-server metadata the flow needs.
type ASMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
}

// Token is the result of a code exchange or refresh.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

var oauthHTTP = &http.Client{Timeout: 20 * time.Second}

// DiscoverAuthServer resolves the authorization server for an MCP server URL.
// It first consults the protected-resource metadata (RFC 9728, path-aware then
// root), and falls back to treating the MCP origin itself as the authorization
// server when no PRM document is published. Returns the AS metadata and the
// canonical resource identifier to bind tokens to (RFC 8707).
func DiscoverAuthServer(ctx context.Context, mcpURL string) (*ASMetadata, string, error) {
	u, err := url.Parse(mcpURL)
	if err != nil || u.Host == "" {
		return nil, "", fmt.Errorf("invalid MCP server URL %q", mcpURL)
	}
	origin := u.Scheme + "://" + u.Host
	resource := mcpURL

	authServer := origin
	var prm struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	prmURLs := []string{
		origin + "/.well-known/oauth-protected-resource" + u.Path,
		origin + "/.well-known/oauth-protected-resource",
	}
	for _, prmURL := range prmURLs {
		if fetchJSON(ctx, prmURL, &prm) == nil && len(prm.AuthorizationServers) > 0 {
			authServer = strings.TrimRight(prm.AuthorizationServers[0], "/")
			if prm.Resource != "" {
				resource = prm.Resource
			}
			break
		}
	}

	meta, err := fetchASMetadata(ctx, authServer)
	if err != nil {
		return nil, "", err
	}
	return meta, resource, nil
}

// fetchASMetadata tries the RFC 8414 well-known locations (path-aware and
// root) and the OIDC discovery document.
func fetchASMetadata(ctx context.Context, authServer string) (*ASMetadata, error) {
	u, err := url.Parse(authServer)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid authorization server URL %q", authServer)
	}
	origin := u.Scheme + "://" + u.Host
	candidates := []string{
		origin + "/.well-known/oauth-authorization-server" + u.Path,
		origin + "/.well-known/oauth-authorization-server",
		origin + "/.well-known/openid-configuration" + u.Path,
		origin + "/.well-known/openid-configuration",
	}
	var lastErr error
	for _, c := range candidates {
		var meta ASMetadata
		if err := fetchJSON(ctx, c, &meta); err != nil {
			lastErr = err
			continue
		}
		if meta.AuthorizationEndpoint != "" && meta.TokenEndpoint != "" {
			return &meta, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable metadata document")
	}
	return nil, fmt.Errorf("authorization server metadata discovery failed for %s: %w", authServer, lastErr)
}

// RegisterClient performs RFC 7591 dynamic client registration as a public
// client (PKCE, token_endpoint_auth_method "none"). Returns client_id and, if
// the server issued one anyway, client_secret.
func RegisterClient(ctx context.Context, registrationEndpoint, redirectURI, clientName string) (string, string, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name":                clientName,
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := oauthHTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("client registration: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", "", fmt.Errorf("client registration failed: %s: %s", res.Status, truncateStr(string(raw), 300))
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ClientID == "" {
		return "", "", fmt.Errorf("client registration returned no client_id")
	}
	return out.ClientID, out.ClientSecret, nil
}

// NewPKCE returns a fresh (verifier, S256 challenge) pair.
func NewPKCE() (string, string) {
	raw := make([]byte, 48)
	_, _ = rand.Read(raw)
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL builds the browser authorization URL for the code+PKCE grant.
func AuthorizeURL(meta *ASMetadata, clientID, redirectURI, state, challenge, resource, scope string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if resource != "" {
		q.Set("resource", resource)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	sep := "?"
	if strings.Contains(meta.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return meta.AuthorizationEndpoint + sep + q.Encode()
}

// ExchangeCode redeems an authorization code at the token endpoint.
func ExchangeCode(ctx context.Context, tokenEndpoint, clientID, clientSecret, code, redirectURI, verifier, resource string) (*Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	if resource != "" {
		form.Set("resource", resource)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	return tokenRequest(ctx, tokenEndpoint, form)
}

// RefreshToken redeems a refresh token for a fresh access token.
func RefreshToken(ctx context.Context, tokenEndpoint, clientID, clientSecret, refreshToken, resource string) (*Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if resource != "" {
		form.Set("resource", resource)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	return tokenRequest(ctx, tokenEndpoint, form)
}

func tokenRequest(ctx context.Context, tokenEndpoint string, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("token request failed: %s: %s", res.Status, truncateStr(string(raw), 300))
	}
	var t Token
	if err := json.Unmarshal(raw, &t); err != nil || t.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	return &t, nil
}

func fetchJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", rawURL, res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
