package handler

import (
	"context"
	"encoding/json"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

// httpChannelProvider is stateless — an http channel has no adapter, no
// daemon, and no capabilities to attach.
type httpChannelProvider struct{}

func (httpChannelProvider) Type() string { return "http" }

func (httpChannelProvider) NormalizeConfig(raw json.RawMessage) domain.GatewayChannelConfig {
	return normalizeCommonConfig(raw)
}

func (httpChannelProvider) AttachCapabilities(context.Context, string) error { return nil }
