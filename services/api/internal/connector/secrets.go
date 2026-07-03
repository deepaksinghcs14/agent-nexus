package connector

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connector configs carry provider credentials (GitHub PATs, Confluence API
// tokens). They are AES-256-GCM encrypted at rest with the instance
// ENCRYPTION_KEY — the same key protecting runner_credentials and MCP OAuth
// tokens — and never returned by the API. Values are marked with a prefix so
// reads can tell ciphertext from legacy plaintext, which keeps the startup
// backfill idempotent and pre-encryption rows working until it runs.

// secretConfigKeys are config fields treated as credentials wherever a
// connector config is written, read, or serialized.
var secretConfigKeys = []string{"token", "api_token", "api_key", "password", "secret"}

const encPrefix = "enc:v1:"

// EncryptConfigSecrets returns raw with every secret config field encrypted.
// Already-encrypted values pass through unchanged. On an unusable key the
// input is returned as-is (callers keep working; the value stays plaintext).
func EncryptConfigSecrets(key []byte, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(key) != 32 {
		return raw
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil || cfg == nil {
		return raw
	}
	changed := false
	for _, k := range secretConfigKeys {
		v, ok := cfg[k].(string)
		if !ok || v == "" || strings.HasPrefix(v, encPrefix) {
			continue
		}
		enc, err := encrypt.Encrypt(key, v)
		if err != nil {
			slog.Warn("connector config secret encryption failed; storing plaintext", "field", k, "error", err)
			continue
		}
		cfg[k] = encPrefix + enc
		changed = true
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return out
}

// DecryptConfigSecrets decrypts encrypted secret fields of cfg in place.
// Plaintext (legacy) values are left untouched, so reads work before and
// after the backfill.
func DecryptConfigSecrets(key []byte, cfg map[string]any) {
	if cfg == nil {
		return
	}
	for _, k := range secretConfigKeys {
		v, ok := cfg[k].(string)
		if !ok || !strings.HasPrefix(v, encPrefix) {
			continue
		}
		if len(key) != 32 {
			slog.Warn("connector config has encrypted secret but ENCRYPTION_KEY is unusable", "field", k)
			continue
		}
		dec, err := encrypt.Decrypt(key, strings.TrimPrefix(v, encPrefix))
		if err != nil {
			slog.Warn("connector config secret decryption failed", "field", k, "error", err)
			continue
		}
		cfg[k] = dec
	}
}

// RedactConfigSecrets returns raw with secret fields replaced by a
// placeholder — API responses must expose neither plaintext nor ciphertext.
func RedactConfigSecrets(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil || cfg == nil {
		return raw
	}
	changed := false
	for _, k := range secretConfigKeys {
		if v, ok := cfg[k].(string); ok && v != "" {
			cfg[k] = "•••"
			changed = true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return out
}

// EncryptLegacyConfigs encrypts plaintext secrets in existing connector rows.
// Runs at every startup; idempotent (the enc: prefix marks done rows).
func EncryptLegacyConfigs(ctx context.Context, pool *pgxpool.Pool, key []byte) (int, error) {
	if len(key) != 32 {
		return 0, nil
	}
	rows, err := pool.Query(ctx, `SELECT id::text, config FROM connectors WHERE config IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type row struct {
		id  string
		cfg json.RawMessage
	}
	var all []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.cfg) == nil {
			all = append(all, r)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	updated := 0
	for _, r := range all {
		enc := EncryptConfigSecrets(key, r.cfg)
		if string(enc) == string(r.cfg) {
			continue
		}
		if _, err := pool.Exec(ctx, `UPDATE connectors SET config=$2, updated_at=NOW() WHERE id=$1::uuid`, r.id, enc); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
