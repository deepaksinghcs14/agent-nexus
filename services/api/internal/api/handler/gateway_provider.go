package handler

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

// ChannelProvider covers the actual closed-set decision points that vary by
// gateway channel type: validation (via the registry key), config
// normalization, and post-create/update capability wiring. WhatsApp-only
// lifecycle surface (QR login, contacts, owner commands, adapter sync) is
// NOT part of this interface — it stays on GatewayHandler's WhatsApp-specific
// methods, gated by loadWhatsAppChannel, same as before. A hypothetical third
// channel type would add its own equivalent surface, same as WhatsApp did.
type ChannelProvider interface {
	Type() string
	NormalizeConfig(raw json.RawMessage) domain.GatewayChannelConfig
	AttachCapabilities(ctx context.Context, agentID string) error
}

var channelProviders sync.Map // channel type -> ChannelProvider

func registerChannelProvider(p ChannelProvider) { channelProviders.Store(p.Type(), p) }

func getChannelProvider(channelType string) (ChannelProvider, bool) {
	v, ok := channelProviders.Load(channelType)
	if !ok {
		return nil, false
	}
	return v.(ChannelProvider), true
}

func registeredChannelTypes() []string {
	var types []string
	channelProviders.Range(func(k, _ any) bool {
		types = append(types, k.(string))
		return true
	})
	sort.Strings(types)
	return types
}

// normalizeCommonConfig applies the defaults shared by every channel type.
// Providers with type-specific defaults (e.g. WhatsApp's adapter URL) call
// this first and layer their own on top.
func normalizeCommonConfig(raw json.RawMessage) domain.GatewayChannelConfig {
	cfg := domain.GatewayChannelConfig{}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.AccountID == "" {
		cfg.AccountID = "default"
	}
	if cfg.DMPolicy == "" {
		cfg.DMPolicy = "pairing"
	}
	if cfg.SessionScope == "" {
		cfg.SessionScope = "per-channel-peer"
	}
	if cfg.GroupPolicy == "" {
		cfg.GroupPolicy = "disabled"
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 50
	}
	return cfg
}
