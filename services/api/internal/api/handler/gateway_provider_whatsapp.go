package handler

import (
	"context"
	"encoding/json"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// whatsappChannelProvider captures the config-level default (adapter URL)
// that only NewGatewayHandler knows, since it comes from *config.Config.
type whatsappChannelProvider struct {
	pool              *pgxpool.Pool
	defaultAdapterURL string
}

func (p whatsappChannelProvider) Type() string { return "whatsapp" }

func (p whatsappChannelProvider) NormalizeConfig(raw json.RawMessage) domain.GatewayChannelConfig {
	cfg := normalizeCommonConfig(raw)
	if cfg.AdapterURL == "" {
		cfg.AdapterURL = p.defaultAdapterURL
	}
	if !hasJSONKey(raw, "assistant_enabled") {
		cfg.AssistantEnabled = true
	}
	if !hasJSONKey(raw, "chat_approvals_enabled") {
		cfg.ChatApprovalsEnabled = true
	}
	return cfg
}

func (p whatsappChannelProvider) AttachCapabilities(ctx context.Context, agentID string) error {
	return AttachWhatsAppCapabilities(ctx, p.pool, agentID)
}
