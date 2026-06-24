package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	repo *repository.GatewayRepository
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, repo: repository.NewGatewayRepository(pool)}
}

func ParseConfig(raw json.RawMessage, defaultAdapterURL string) domain.GatewayChannelConfig {
	cfg := domain.GatewayChannelConfig{}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.AccountID == "" {
		cfg.AccountID = "default"
	}
	if cfg.AdapterURL == "" {
		cfg.AdapterURL = defaultAdapterURL
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

type SendRequest struct {
	Channel   domain.GatewayChannel
	Config    domain.GatewayChannelConfig
	AccountID string
	PeerKind  string
	PeerID    string
	Body      string
	Source    string
	SessionID string
	RunID     string
}

func (s *Service) SendWhatsApp(ctx context.Context, req SendRequest) (*domain.GatewayOutboundMessage, error) {
	accountID := req.AccountID
	if accountID == "" {
		accountID = req.Config.AccountID
	}
	peerKind := req.PeerKind
	if peerKind == "" {
		peerKind = "direct"
	}
	source := req.Source
	if source == "" {
		source = "assistant_reply"
	}
	out := &domain.GatewayOutboundMessage{
		WorkspaceID: req.Channel.WorkspaceID,
		ChannelID:   req.Channel.ID,
		SessionID:   req.SessionID,
		RunID:       req.RunID,
		AccountID:   accountID,
		PeerKind:    peerKind,
		PeerID:      req.PeerID,
		Body:        req.Body,
		Source:      source,
		Status:      "pending",
	}
	if err := s.repo.CreateOutbound(ctx, out); err != nil {
		return nil, fmt.Errorf("gateway: create outbound record: %w", err)
	}
	payload := map[string]any{"peer": map[string]string{"kind": peerKind, "id": req.PeerID}, "text": req.Body}
	_, err := adapterPost(ctx, req.Config.AdapterURL, "/accounts/"+url.PathEscape(accountID)+"/send", payload)
	if err != nil {
		_ = s.repo.MarkOutbound(ctx, out.ID, "failed", err.Error())
		_ = s.LogEvent(ctx, req.Channel, req.SessionID, req.RunID, "delivery_failed", "", map[string]any{"error": err.Error(), "peer_id": req.PeerID})
		out.Status = "failed"
		out.LastError = err.Error()
		return out, err
	}
	_ = s.repo.MarkOutbound(ctx, out.ID, "sent", "")
	_ = s.LogEvent(ctx, req.Channel, req.SessionID, req.RunID, "delivery_succeeded", "", map[string]any{"peer_id": req.PeerID})
	out.Status = "sent"
	return out, nil
}

func (s *Service) LogEvent(ctx context.Context, c domain.GatewayChannel, sessionID, runID, typ, providerMessageID string, payload any) error {
	b, _ := json.Marshal(payload)
	return s.repo.CreateEvent(ctx, &domain.GatewayEvent{
		WorkspaceID: c.WorkspaceID, ChannelID: c.ID, SessionID: sessionID, RunID: runID,
		EventType: typ, ProviderMessageID: providerMessageID, Payload: b,
	})
}

func PhoneToJID(phone string) string {
	p := strings.TrimPrefix(NormalizePhone(phone), "+")
	if p == "" {
		return ""
	}
	return p + "@s.whatsapp.net"
}

func NormalizePhone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	prefix := ""
	if strings.HasPrefix(s, "+") {
		prefix = "+"
	}
	var b strings.Builder
	b.WriteString(prefix)
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func adapterPost(ctx context.Context, base, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return adapterDo(req)
}

func adapterDo(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("adapter returned %s: %s", res.Status, string(body))
	}
	return body, nil
}
