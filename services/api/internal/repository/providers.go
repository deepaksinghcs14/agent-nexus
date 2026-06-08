package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/agentNexus/agent-nexus/services/api/pkg/encrypt"
)

type ProviderRepository struct {
	pool *pgxpool.Pool
}

func NewProviderRepository(pool *pgxpool.Pool) *ProviderRepository {
	return &ProviderRepository{pool: pool}
}

func (r *ProviderRepository) List(ctx context.Context, workspaceID string) ([]domain.ProviderCredential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, workspace_id::text, provider, display_name, base_url, is_active,
		        COALESCE(auth_type,'api_key'), oauth_token_expiry,
		        created_by::text, created_at, updated_at
		 FROM provider_credentials WHERE workspace_id = $1::uuid ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []domain.ProviderCredential
	for rows.Next() {
		var c domain.ProviderCredential
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.DisplayName, &c.BaseURL,
			&c.IsActive, &c.AuthType, &c.OAuthTokenExpiry, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	if creds == nil {
		creds = []domain.ProviderCredential{}
	}
	return creds, rows.Err()
}

func (r *ProviderRepository) Create(ctx context.Context, p *domain.ProviderCredential, encryptedKey string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO provider_credentials
		 (id, workspace_id, provider, display_name, encrypted_key, base_url, is_active, auth_type, created_by, created_at, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'api_key', $8::uuid, NOW(), NOW())
		 ON CONFLICT (workspace_id, provider) DO UPDATE
		 SET display_name=$4, encrypted_key=$5, base_url=$6, is_active=$7, auth_type='api_key',
		     oauth_access_token=NULL, oauth_refresh_token=NULL, oauth_token_expiry=NULL, oauth_scopes=NULL,
		     updated_at=NOW()`,
		p.ID, p.WorkspaceID, p.Provider, p.DisplayName, encryptedKey, p.BaseURL, p.IsActive, p.CreatedBy)
	return err
}

func (r *ProviderRepository) Get(ctx context.Context, id string) (*domain.ProviderCredential, string, error) {
	var c domain.ProviderCredential
	var encKey string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, provider, display_name, encrypted_key, base_url, is_active,
		        COALESCE(auth_type,'api_key'), oauth_token_expiry,
		        created_by::text, created_at, updated_at
		 FROM provider_credentials WHERE id = $1::uuid`, id).
		Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.DisplayName, &encKey, &c.BaseURL, &c.IsActive,
			&c.AuthType, &c.OAuthTokenExpiry, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, "", fmt.Errorf("provider not found")
	}
	return &c, encKey, err
}

func (r *ProviderRepository) GetActiveByProvider(ctx context.Context, workspaceID, providerName string) (*domain.ProviderCredential, string, error) {
	var c domain.ProviderCredential
	var encKey string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, provider, display_name, encrypted_key, base_url, is_active,
		        COALESCE(auth_type,'api_key'), oauth_token_expiry,
		        created_by::text, created_at, updated_at
		 FROM provider_credentials
		 WHERE workspace_id = $1::uuid AND provider = $2 AND is_active = true
		 ORDER BY updated_at DESC
		 LIMIT 1`, workspaceID, providerName).
		Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.DisplayName, &encKey, &c.BaseURL, &c.IsActive,
			&c.AuthType, &c.OAuthTokenExpiry, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, "", fmt.Errorf("provider credential not found")
	}
	return &c, encKey, err
}

func (r *ProviderRepository) Update(ctx context.Context, p *domain.ProviderCredential, encryptedKey string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE provider_credentials SET display_name=$2, encrypted_key=$3, base_url=$4, is_active=$5, updated_at=NOW()
		 WHERE id=$1::uuid`,
		p.ID, p.DisplayName, encryptedKey, p.BaseURL, p.IsActive)
	return err
}

func (r *ProviderRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM provider_credentials WHERE id = $1::uuid`, id)
	return err
}

// UpsertOAuthCredential stores or replaces an OAuth credential for a workspace+provider pair.
// Tokens must already be AES-256-GCM encrypted.
func (r *ProviderRepository) UpsertOAuthCredential(ctx context.Context, workspaceID, provider, displayName, encAccessToken, encRefreshToken string, expiry time.Time, scopes []string, createdBy string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO provider_credentials
		 (id, workspace_id, provider, display_name, encrypted_key, base_url, is_active,
		  auth_type, oauth_access_token, oauth_refresh_token, oauth_token_expiry, oauth_scopes, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1::uuid, $2, $3, '', '', true,
		  'oauth', $4, $5, $6, $7, $8::uuid, NOW(), NOW())
		 ON CONFLICT (workspace_id, provider) DO UPDATE
		 SET display_name=$3, encrypted_key='', is_active=true, auth_type='oauth',
		     oauth_access_token=$4, oauth_refresh_token=$5, oauth_token_expiry=$6, oauth_scopes=$7,
		     updated_at=NOW()`,
		workspaceID, provider, displayName, encAccessToken, encRefreshToken, expiry, scopes, createdBy)
	return err
}

// GetOAuthTokens returns the encrypted access token, encrypted refresh token, and expiry for a credential.
func (r *ProviderRepository) GetOAuthTokens(ctx context.Context, credID string) (encAccess, encRefresh string, expiry time.Time, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(oauth_access_token,''), COALESCE(oauth_refresh_token,''), COALESCE(oauth_token_expiry, NOW())
		 FROM provider_credentials WHERE id=$1::uuid`, credID).
		Scan(&encAccess, &encRefresh, &expiry)
	return
}

// UpdateOAuthAccessToken replaces the stored access token and its expiry.
// encAccessToken must already be AES-256-GCM encrypted.
func (r *ProviderRepository) UpdateOAuthAccessToken(ctx context.Context, credID, encAccessToken string, expiry time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE provider_credentials SET oauth_access_token=$2, oauth_token_expiry=$3, updated_at=NOW() WHERE id=$1::uuid`,
		credID, encAccessToken, expiry)
	return err
}

// RefreshGoogleToken exchanges a refresh token for a new access token using the Google token endpoint.
// Returns the new plaintext access token and expiry.
func RefreshGoogleToken(ctx context.Context, clientID, clientSecret, refreshToken string) (accessToken string, expiry time.Time, err error) {
	vals := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(vals.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("google token refresh: %s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", time.Time{}, err
	}
	return tok.AccessToken, time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second), nil
}

// GetDecryptedAccessToken returns the active OAuth access token for a credential, refreshing if needed.
// Returns the plaintext access token ready to use.
func (r *ProviderRepository) GetDecryptedAccessToken(ctx context.Context, credID, encKey, clientID, clientSecret string) (string, error) {
	encAccess, encRefresh, expiry, err := r.GetOAuthTokens(ctx, credID)
	if err != nil {
		return "", fmt.Errorf("failed to load oauth tokens: %w", err)
	}

	// Refresh if within 5 minutes of expiry
	if time.Until(expiry) < 5*time.Minute {
		plainRefresh, err := encrypt.Decrypt([]byte(encKey), encRefresh)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt refresh token: %w", err)
		}
		newAccess, newExpiry, err := RefreshGoogleToken(ctx, clientID, clientSecret, plainRefresh)
		if err != nil {
			return "", fmt.Errorf("failed to refresh google token: %w", err)
		}
		encNewAccess, err := encrypt.Encrypt([]byte(encKey), newAccess)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt new access token: %w", err)
		}
		if err := r.UpdateOAuthAccessToken(ctx, credID, encNewAccess, newExpiry); err != nil {
			return "", fmt.Errorf("failed to store refreshed token: %w", err)
		}
		return newAccess, nil
	}

	return encrypt.Decrypt([]byte(encKey), encAccess)
}

// CreateOAuthState stores a CSRF state token for 10 minutes.
func (r *ProviderRepository) CreateOAuthState(ctx context.Context, state, workspaceID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO oauth_states(state, workspace_id, user_id, expires_at) VALUES($1, $2::uuid, $3::uuid, NOW() + INTERVAL '10 minutes')`,
		state, workspaceID, userID)
	return err
}

// ConsumeOAuthState validates and deletes the state, returning the workspace and user IDs.
func (r *ProviderRepository) ConsumeOAuthState(ctx context.Context, state string) (workspaceID, userID string, err error) {
	err = r.pool.QueryRow(ctx,
		`DELETE FROM oauth_states WHERE state=$1 AND expires_at > NOW() RETURNING workspace_id::text, user_id::text`,
		state).Scan(&workspaceID, &userID)
	if err == pgx.ErrNoRows {
		return "", "", fmt.Errorf("invalid or expired oauth state")
	}
	return
}
