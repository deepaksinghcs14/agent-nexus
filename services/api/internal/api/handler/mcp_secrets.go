package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MCP server configs carry credentials: the HTTP auth token and, for stdio
// servers spawned from the preset catalog, arbitrary environment variables
// (JIRA_API_TOKEN, SLACK_BOT_TOKEN, …). Both are AES-256-GCM encrypted at
// rest with the instance ENCRYPTION_KEY, using the same enc:v1: marker
// convention as connector configs so ciphertext and legacy plaintext coexist.
// Every env value is treated as secret — the portal collects them precisely
// because they are credentials.

const mcpEncPrefix = "enc:v1:"

// encryptMCPConfig returns raw with the token field and all env values
// encrypted. Already-encrypted values pass through; on an unusable key the
// input is returned unchanged.
func encryptMCPConfig(key []byte, raw json.RawMessage) json.RawMessage {
	return transformMCPConfig(raw, func(v string) string {
		if strings.HasPrefix(v, mcpEncPrefix) || len(key) != 32 {
			return v
		}
		enc, err := encrypt.Encrypt(key, v)
		if err != nil {
			slog.Warn("mcp config secret encryption failed; storing plaintext", "error", err)
			return v
		}
		return mcpEncPrefix + enc
	})
}

// decryptMCPConfig decrypts encrypted values; legacy plaintext passes through.
func decryptMCPConfig(key []byte, raw json.RawMessage) json.RawMessage {
	return transformMCPConfig(raw, func(v string) string {
		if !strings.HasPrefix(v, mcpEncPrefix) {
			return v
		}
		if len(key) != 32 {
			slog.Warn("mcp config has encrypted secret but ENCRYPTION_KEY is unusable")
			return v
		}
		dec, err := encrypt.Decrypt(key, strings.TrimPrefix(v, mcpEncPrefix))
		if err != nil {
			slog.Warn("mcp config secret decryption failed", "error", err)
			return v
		}
		return dec
	})
}

// redactMCPConfig blanks secret values for API responses — env keys stay
// visible (the UI shows which variables are set) but values never leave the
// server in either plaintext or ciphertext form.
func redactMCPConfig(raw json.RawMessage) json.RawMessage {
	return transformMCPConfig(raw, func(string) string { return "•••" })
}

// transformMCPConfig applies fn to the token field and every env value.
func transformMCPConfig(raw json.RawMessage, fn func(string) string) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil || cfg == nil {
		return raw
	}
	changed := false
	if v, ok := cfg["token"].(string); ok && v != "" {
		if nv := fn(v); nv != v {
			cfg["token"] = nv
			changed = true
		}
	}
	if env, ok := cfg["env"].(map[string]any); ok {
		for k, ev := range env {
			if v, ok := ev.(string); ok && v != "" {
				if nv := fn(v); nv != v {
					env[k] = nv
					changed = true
				}
			}
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

// EncryptLegacyMCPConfigs encrypts plaintext secrets in existing mcp_servers
// rows. Runs at every startup; idempotent via the enc: prefix marker.
func EncryptLegacyMCPConfigs(ctx context.Context, pool *pgxpool.Pool, key []byte) (int, error) {
	if len(key) != 32 {
		return 0, nil
	}
	rows, err := pool.Query(ctx, `SELECT id::text, config FROM mcp_servers WHERE config IS NOT NULL`)
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
		enc := encryptMCPConfig(key, r.cfg)
		if string(enc) == string(r.cfg) {
			continue
		}
		if _, err := pool.Exec(ctx, `UPDATE mcp_servers SET config=$2, updated_at=NOW() WHERE id=$1::uuid`, r.id, enc); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
