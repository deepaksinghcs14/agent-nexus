package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	gatewayservice "github.com/deepaksingh/agent-nexus/services/api/internal/gateway"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GatewayHandler struct {
	pool   *pgxpool.Pool
	cfg    *config.Config
	repo   *repository.GatewayRepository
	invoke *InvokeHandler
}

func NewGatewayHandler(pool *pgxpool.Pool, cfg *config.Config, invoke *InvokeHandler) *GatewayHandler {
	return &GatewayHandler{pool: pool, cfg: cfg, repo: repository.NewGatewayRepository(pool), invoke: invoke}
}

func (h *GatewayHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListChannels(r.Context(), ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway channels"))
		return
	}
	for i := range list {
		list[i].Config = redactGatewayConfig(list[i].Config)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		AgentID     string          `json:"agent_id"`
		ChannelType string          `json:"channel_type"`
		Config      json.RawMessage `json:"config"`
		IsActive    *bool           `json:"is_active"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" || body.AgentID == "" || body.ChannelType == "" {
		errs.Write(w, errs.BadRequest("name, agent_id, and channel_type are required"))
		return
	}
	if body.ChannelType != "whatsapp" && body.ChannelType != "http" {
		errs.Write(w, errs.BadRequest("channel_type must be whatsapp or http"))
		return
	}
	cfg := h.normalizeGatewayConfig(body.Config, body.ChannelType)
	cfgBytes, _ := json.Marshal(cfg)
	active := true
	if body.IsActive != nil {
		active = *body.IsActive
	}
	c := &domain.GatewayChannel{
		ID:          uuid.NewString(),
		WorkspaceID: ws,
		AgentID:     body.AgentID,
		Name:        body.Name,
		Description: body.Description,
		ChannelType: body.ChannelType,
		Config:      cfgBytes,
		IsActive:    active,
		CreatedBy:   uid,
	}
	if err := h.repo.CreateChannel(r.Context(), c); err != nil {
		errs.Write(w, errs.Internal("failed to create gateway channel"))
		return
	}
	_ = h.repo.UpsertAccount(r.Context(), &domain.GatewayChannelAccount{
		WorkspaceID: ws, ChannelID: c.ID, AccountID: cfg.AccountID, Status: "disconnected",
	})
	if c.ChannelType == "whatsapp" {
		_ = AttachWhatsAppCapabilities(r.Context(), h.pool, c.AgentID)
	}
	writeAudit(r, h.pool, "gateway_channel.created", "gateway_channel", c.ID)
	c.Config = redactGatewayConfig(c.Config)
	errs.WriteJSON(w, http.StatusCreated, c)
}

func (h *GatewayHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	c, err := h.repo.GetChannelInWorkspace(r.Context(), chi.URLParam(r, "id"), ws)
	if err != nil {
		errs.Write(w, errs.NotFound("gateway channel not found"))
		return
	}
	c.Config = redactGatewayConfig(c.Config)
	errs.WriteJSON(w, http.StatusOK, c)
}

func (h *GatewayHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	c, err := h.repo.GetChannelInWorkspace(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("gateway channel not found"))
		return
	}
	var body struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		AgentID     *string         `json:"agent_id"`
		Config      json.RawMessage `json:"config"`
		IsActive    *bool           `json:"is_active"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.Name != nil {
		c.Name = *body.Name
	}
	if body.Description != nil {
		c.Description = *body.Description
	}
	if body.AgentID != nil {
		c.AgentID = *body.AgentID
	}
	if len(body.Config) > 0 {
		cfg := h.normalizeGatewayConfig(body.Config, c.ChannelType)
		c.Config, _ = json.Marshal(cfg)
	}
	if body.IsActive != nil {
		c.IsActive = *body.IsActive
	}
	if err := h.repo.UpdateChannel(r.Context(), &c); err != nil {
		errs.Write(w, errs.Internal("failed to update gateway channel"))
		return
	}
	if c.ChannelType == "whatsapp" {
		h.syncAdapterConfig(r.Context(), c)
		_ = AttachWhatsAppCapabilities(r.Context(), h.pool, c.AgentID)
	}
	writeAudit(r, h.pool, "gateway_channel.updated", "gateway_channel", id)
	c.Config = redactGatewayConfig(c.Config)
	errs.WriteJSON(w, http.StatusOK, c)
}

func (h *GatewayHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteChannel(r.Context(), id, ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete gateway channel"))
		return
	}
	writeAudit(r, h.pool, "gateway_channel.deleted", "gateway_channel", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListSessions(r.Context(), ws, r.URL.Query().Get("channel_id"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway sessions"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	if err := h.repo.DeleteSession(r.Context(), chi.URLParam(r, "id"), ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete gateway session"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListEvents(r.Context(), ws, r.URL.Query().Get("channel_id"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway events"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ListPairings(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListPairings(r.Context(), ws, r.URL.Query().Get("channel_id"), r.URL.Query().Get("status"))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway pairings"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ApprovePairing(w http.ResponseWriter, r *http.Request) {
	h.updatePairing(w, r, "approved")
}

func (h *GatewayHandler) RejectPairing(w http.ResponseWriter, r *http.Request) {
	h.updatePairing(w, r, "rejected")
}

func (h *GatewayHandler) updatePairing(w http.ResponseWriter, r *http.Request, status string) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	p, err := h.repo.UpdatePairingStatus(r.Context(), chi.URLParam(r, "id"), ws, status)
	if err != nil {
		errs.Write(w, errs.NotFound("pairing request not found"))
		return
	}
	if status == "approved" {
		_ = h.createTrustedContactFromPairing(r.Context(), p)
	}
	errs.WriteJSON(w, http.StatusOK, p)
}

func (h *GatewayHandler) ListOutbox(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListOutbox(r.Context(), ws, r.URL.Query().Get("channel_id"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway outbox"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListContacts(r.Context(), ws, r.URL.Query().Get("channel_id"))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway contacts"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ListReminders(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListReminders(r.Context(), ws, r.URL.Query().Get("channel_id"), r.URL.Query().Get("status"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway reminders"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ListEscalations(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListEscalations(r.Context(), ws, r.URL.Query().Get("channel_id"), r.URL.Query().Get("status"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway escalations"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) ApproveEscalation(w http.ResponseWriter, r *http.Request) {
	h.resolveEscalation(w, r, "approved")
}

func (h *GatewayHandler) RejectEscalation(w http.ResponseWriter, r *http.Request) {
	h.resolveEscalation(w, r, "rejected")
}

func (h *GatewayHandler) resolveEscalation(w http.ResponseWriter, r *http.Request, status string) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	e, err := h.repo.ResolveEscalationByID(r.Context(), chi.URLParam(r, "id"), ws, status, uid)
	if err != nil {
		errs.Write(w, errs.NotFound("pending escalation not found"))
		return
	}
	writeAudit(r, h.pool, "gateway_escalation."+status, "gateway_escalation", e.ID)
	errs.WriteJSON(w, http.StatusOK, e)
}

func (h *GatewayHandler) CreateContact(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var body domain.GatewayContact
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.ChannelID == "" || body.DisplayName == "" || body.Role == "" {
		errs.Write(w, errs.BadRequest("channel_id, display_name, and role are required"))
		return
	}
	if !validGatewayContactRole(body.Role) {
		errs.Write(w, errs.BadRequest("role must be owner, trusted, or blocked"))
		return
	}
	channel, err := h.repo.GetChannelInWorkspace(r.Context(), body.ChannelID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("gateway channel not found"))
		return
	}
	cfg := h.parseGatewayConfig(channel.Config, channel.ChannelType)
	body.WorkspaceID = ws
	body.AccountID = defaultString(body.AccountID, cfg.AccountID)
	body.Alias = defaultString(body.Alias, aliasFromName(body.DisplayName))
	body.PhoneNumber = normalizeGatewayPhone(body.PhoneNumber)
	body.WhatsAppJID = defaultString(body.WhatsAppJID, jidFromPhone(body.PhoneNumber))
	if !body.AutoReplyEnabled && r.URL.Query().Get("explicit_auto_reply") == "" {
		body.AutoReplyEnabled = true
	}
	if err := h.repo.CreateContact(r.Context(), &body); err != nil {
		errs.Write(w, errs.Internal("failed to create gateway contact"))
		return
	}
	if body.PhoneNumber != "" && channel.ChannelType == "whatsapp" {
		go h.syncChannelLIDs(body.ChannelID, cfg)
	}
	writeAudit(r, h.pool, "gateway_contact.created", "gateway_contact", body.ID)
	errs.WriteJSON(w, http.StatusCreated, body)
}

func (h *GatewayHandler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	c, err := h.repo.GetContact(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("gateway contact not found"))
		return
	}
	var body domain.GatewayContact
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.DisplayName != "" {
		c.DisplayName = body.DisplayName
	}
	if body.Alias != "" {
		c.Alias = body.Alias
	}
	if body.PhoneNumber != "" {
		c.PhoneNumber = normalizeGatewayPhone(body.PhoneNumber)
		c.WhatsAppJID = jidFromPhone(c.PhoneNumber)
	}
	if body.WhatsAppJID != "" {
		c.WhatsAppJID = body.WhatsAppJID
	}
	if body.Role != "" {
		if !validGatewayContactRole(body.Role) {
			errs.Write(w, errs.BadRequest("role must be owner, trusted, or blocked"))
			return
		}
		c.Role = body.Role
	}
	c.AgentID = body.AgentID
	c.AutoReplyEnabled = body.AutoReplyEnabled
	if err := h.repo.UpdateContact(r.Context(), &c); err != nil {
		errs.Write(w, errs.Internal("failed to update gateway contact"))
		return
	}
	if c.PhoneNumber != "" {
		if ch, err := h.repo.GetChannelInWorkspace(r.Context(), c.ChannelID, ws); err == nil && ch.ChannelType == "whatsapp" {
			chCfg := h.parseGatewayConfig(ch.Config, ch.ChannelType)
			go h.syncChannelLIDs(c.ChannelID, chCfg)
		}
	}
	writeAudit(r, h.pool, "gateway_contact.updated", "gateway_contact", id)
	errs.WriteJSON(w, http.StatusOK, c)
}

func (h *GatewayHandler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteContact(r.Context(), id, ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete gateway contact"))
		return
	}
	writeAudit(r, h.pool, "gateway_contact.deleted", "gateway_contact", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) AdapterStatus(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	body, err := adapterGet(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/status")
	if err != nil {
		_ = h.repo.UpsertAccount(r.Context(), &domain.GatewayChannelAccount{
			WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AccountID: cfg.AccountID, Status: "error", LastError: err.Error(),
		})
		errs.Write(w, errs.BadRequest("adapter status failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

func (h *GatewayHandler) AdapterLoginStart(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	phones, _ := h.repo.ListContactPhones(r.Context(), c.ID, cfg.AccountID)
	body, err := adapterPost(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/start", map[string]any{
		"channel_id":        c.ID,
		"callback_url":      strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/gateway/whatsapp/" + c.ID,
		"self_chat_enabled": cfg.SelfChatEnabled,
		"contact_phones":    phones,
	})
	if err != nil {
		errs.Write(w, errs.BadRequest("adapter login failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

// syncChannelLIDs re-pushes contact phones to the adapter so it can resolve LID→phone
// mappings via onWhatsApp(). Called after contact create/update because new contacts
// may have LID JIDs that aren't in the adapter's cache yet.
func (h *GatewayHandler) syncChannelLIDs(channelID string, cfg domain.GatewayChannelConfig) {
	phones, err := h.repo.ListContactPhones(context.Background(), channelID, cfg.AccountID)
	if err != nil || len(phones) == 0 {
		return
	}
	_, _ = adapterPost(context.Background(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/start", map[string]any{
		"contact_phones": phones,
	})
}

func (h *GatewayHandler) syncAdapterConfig(ctx context.Context, c domain.GatewayChannel) {
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	_, _ = adapterPost(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/config", map[string]any{
		"channel_id":        c.ID,
		"callback_url":      strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/gateway/whatsapp/" + c.ID,
		"self_chat_enabled": cfg.SelfChatEnabled,
	})
}

// SyncAllAdapters calls login/start on the WhatsApp adapter for every active WhatsApp
// channel so the adapter reconnects using saved session credentials and receives the
// callbackUrl. Called once at API startup because the adapter loses all in-memory state
// (socket, callbackUrl, selfId) on every container restart.
func (h *GatewayHandler) SyncAllAdapters(ctx context.Context) {
	channels, err := h.repo.ListAllActiveWhatsAppChannels(ctx)
	if err != nil {
		slog.Error("adapter startup sync: list channels failed", "error", err)
		return
	}
	for _, c := range channels {
		cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
		// Fetch contact phone numbers so the adapter can resolve LID JIDs via onWhatsApp().
		// WhatsApp migrated to opaque LID-based JIDs; the adapter needs phone numbers to
		// call onWhatsApp() and build the LID→phone map for contact matching.
		phones, _ := h.repo.ListContactPhones(ctx, c.ID, cfg.AccountID)
		_, err := adapterPost(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/start", map[string]any{
			"channel_id":        c.ID,
			"callback_url":      strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/gateway/whatsapp/" + c.ID,
			"self_chat_enabled": cfg.SelfChatEnabled,
			"contact_phones":    phones,
		})
		if err != nil {
			slog.Warn("adapter startup sync: login/start failed", "channel", c.ID, "error", err)
		} else {
			slog.Info("adapter startup sync: channel reconnected", "channel", c.ID, "account", cfg.AccountID, "contact_phones", len(phones))
		}
	}
}

func (h *GatewayHandler) AdapterQR(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	body, err := adapterGet(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/qr")
	if err != nil {
		errs.Write(w, errs.BadRequest("adapter QR failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

func (h *GatewayHandler) AdapterLogout(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	body, err := adapterPost(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/logout", nil)
	if err != nil {
		errs.Write(w, errs.BadRequest("adapter logout failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

// WhatsAppReceive accepts normalized events from the WhatsApp Web adapter.
func (h *GatewayHandler) WhatsAppReceive(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelId")
	var ev struct {
		Type      string `json:"type"`
		AccountID string `json:"account_id"`
		MessageID string `json:"message_id"`
		Peer      struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"peer"`
		Sender struct {
			ID          string `json:"id"`
			PhoneNumber string `json:"phone_number"`
			DisplayName string `json:"display_name"`
		} `json:"sender"`
		Body       string `json:"body"`
		FromMe     bool   `json:"from_me"`
		ReceivedAt string `json:"received_at"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&ev) != nil || strings.TrimSpace(ev.Body) == "" {
		errs.Write(w, errs.BadRequest("invalid whatsapp adapter event"))
		return
	}
	c, err := h.repo.GetChannel(r.Context(), channelID)
	if err != nil || !c.IsActive || c.ChannelType != "whatsapp" {
		errs.Write(w, errs.NotFound("gateway channel not found"))
		return
	}
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	if ev.AccountID == "" {
		ev.AccountID = cfg.AccountID
	}
	if ev.Peer.Kind == "" {
		ev.Peer.Kind = "direct"
	}
	if ev.Peer.ID == "" {
		ev.Peer.ID = ev.Sender.ID
	}
	accepted, response := h.handleInbound(r.Context(), c, cfg, inboundMessage{
		AccountID: ev.AccountID, ProviderMessageID: ev.MessageID, PeerKind: ev.Peer.Kind,
		PeerID: ev.Peer.ID, SenderID: ev.Sender.ID, SenderPhone: ev.Sender.PhoneNumber,
		SenderName: ev.Sender.DisplayName, FromMe: ev.FromMe, Body: ev.Body, Source: "whatsapp",
	})
	if response != "" {
		go h.sendAdapterMessage(context.Background(), c, cfg, ev.AccountID, ev.Peer.Kind, ev.Peer.ID, response, "", "")
	}
	if !accepted {
		errs.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ignored"})
		return
	}
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
}

func (h *GatewayHandler) HTTPReceive(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelId")
	var body struct {
		Input string `json:"input"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body) != nil || strings.TrimSpace(body.Input) == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}
	c, err := h.repo.GetChannel(r.Context(), channelID)
	if err != nil || !c.IsActive || c.ChannelType != "http" {
		errs.Write(w, errs.NotFound("gateway channel not found"))
		return
	}
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	sender := r.Header.Get("X-Session-ID")
	if sender == "" {
		sender = r.RemoteAddr
	}
	runID, sessionID, convID, err := h.dispatchGatewayRun(context.Background(), c, cfg, inboundMessage{
		AccountID: cfg.AccountID, PeerKind: "direct", PeerID: sender, SenderID: sender, Body: body.Input, Source: "http",
	})
	if err != nil {
		errs.Write(w, errs.Internal("failed to dispatch gateway run"))
		return
	}
	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id": runID, "session_id": sessionID, "conversation_id": convID, "status": "running",
	})
}

type inboundMessage struct {
	AccountID         string
	ProviderMessageID string
	PeerKind          string
	PeerID            string
	SenderID          string
	SenderPhone       string
	SenderName        string
	FromMe            bool
	Body              string
	Source            string
	Contact           *domain.GatewayContact
}

func (h *GatewayHandler) handleInbound(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage) (bool, string) {
	seen, _ := h.repo.HasEventForProviderMessage(ctx, c.ID, msg.ProviderMessageID)
	if seen {
		return false, ""
	}
	contact, decision := h.resolveContact(ctx, c, cfg, msg)
	msg.Contact = contact
	_ = h.logEvent(ctx, c, "", "", "message_received", msg.ProviderMessageID, map[string]any{
		"sender_id": msg.SenderID, "sender_phone": msg.SenderPhone, "peer_id": msg.PeerID,
		"body": msg.Body, "contact_alias": contactAlias(contact), "contact_role": contactRole(contact), "access_decision": decision,
	})
	if response, handled := h.handleOwnerCommand(ctx, c, cfg, msg, contact); handled {
		return false, response
	}
	// Drop messages sent by the WhatsApp account itself (from_me events) unless self-chat
	// is explicitly enabled. Without this guard the agent's own replies arrive as inbound
	// events, match the owner contact, and trigger another run — creating a reply loop.
	if msg.FromMe && !cfg.SelfChatEnabled {
		return false, ""
	}
	if !cfg.AssistantEnabled {
		_ = h.logEvent(ctx, c, "", "", "assistant_disabled", "", map[string]any{"sender_id": msg.SenderID, "contact_alias": contactAlias(contact)})
		return false, ""
	}
	if decision == "unmatched" && cfg.BotModeEnabled {
		contact = h.autoApproveBotSender(ctx, c, cfg, msg)
		msg.Contact = contact
		decision = "contact_allowed"
	}
	if decision == "blocked" {
		_ = h.logEvent(ctx, c, "", "", "contact_blocked", "", map[string]any{"sender_id": msg.SenderID, "contact_alias": contactAlias(contact)})
		return false, ""
	}
	if !h.senderAllowed(cfg, msg, decision) {
		_ = h.logEvent(ctx, c, "", "", "sender_ignored", "", map[string]any{"sender_id": msg.SenderID, "sender_phone": msg.SenderPhone, "decision": decision})
		return false, ""
	}
	runID, sessionID, _, err := h.dispatchGatewayRun(ctx, c, cfg, msg)
	if err != nil {
		_ = h.logEvent(ctx, c, "", "", "run_failed", "", map[string]any{"error": err.Error()})
		return false, ""
	}
	go h.deliverWhenComplete(context.Background(), c, cfg, msg, runID, sessionID)
	return true, ""
}

func (h *GatewayHandler) handleOwnerCommand(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, contact *domain.GatewayContact) (string, bool) {
	if !h.isOwnerSender(ctx, c, cfg, msg, contact) {
		return "", false
	}
	bodyRaw := strings.TrimSpace(msg.Body)
	body := strings.ToLower(bodyRaw)
	parts := strings.Fields(body)
	if len(parts) == 0 {
		return "", false
	}
	if intent, target, ok := parseOwnerNaturalCommand(bodyRaw); ok {
		switch intent {
		case "start_contact":
			return h.handleOwnerContactToggle(ctx, c, cfg, msg, target, true)
		case "stop_contact":
			return h.handleOwnerContactToggle(ctx, c, cfg, msg, target, false)
		case "enable_bot_mode":
			return h.setBotMode(ctx, c, cfg, msg, true)
		case "disable_bot_mode":
			return h.setBotMode(ctx, c, cfg, msg, false)
		case "start_assistant":
			return h.setAssistantEnabled(ctx, c, cfg, msg, true)
		case "stop_assistant":
			return h.setAssistantEnabled(ctx, c, cfg, msg, false)
		}
	}
	switch {
	case body == "stop assistant" || body == "pause assistant" || body == "turn off assistant":
		return h.setAssistantEnabled(ctx, c, cfg, msg, false)
	case body == "start assistant" || body == "resume assistant" || body == "turn on assistant":
		return h.setAssistantEnabled(ctx, c, cfg, msg, true)
	case body == "enable bot mode" || body == "turn on bot mode" || body == "mark channel as bot" || body == "mark this channel as bot":
		return h.setBotMode(ctx, c, cfg, msg, true)
	case body == "disable bot mode" || body == "turn off bot mode" || body == "unmark channel as bot" || body == "unmark this channel as bot":
		return h.setBotMode(ctx, c, cfg, msg, false)
	case parts[0] == "start" && len(parts) >= 2:
		return h.handleOwnerContactToggle(ctx, c, cfg, msg, strings.TrimSpace(bodyRaw[len(parts[0]):]), true)
	case parts[0] == "stop" && len(parts) >= 2:
		return h.handleOwnerContactToggle(ctx, c, cfg, msg, strings.TrimSpace(bodyRaw[len(parts[0]):]), false)
	case (parts[0] == "add" || parts[0] == "allow") && len(parts) >= 2:
		return h.handleOwnerContactToggle(ctx, c, cfg, msg, strings.TrimSpace(bodyRaw[len(parts[0]):]), true)
	case len(parts) >= 2 && (parts[0] == "approve" || parts[0] == "approved"):
		if !cfg.ChatApprovalsEnabled {
			return "Chat approvals are disabled. Enable them with: enable approvals", true
		}
		e, err := h.repo.ResolveEscalationByCode(ctx, c.ID, parts[1], "approved", msg.SenderID)
		if err != nil {
			return "No pending escalation found for code " + strings.ToUpper(parts[1]) + ".", true
		}
		_ = h.logEvent(ctx, c, e.SessionID, e.RunID, "escalation_approved", "", map[string]any{"code": e.ApprovalCode, "by": msg.SenderID})
		return "Approved escalation " + e.ApprovalCode + ".", true
	case len(parts) >= 2 && (parts[0] == "reject" || parts[0] == "rejected" || parts[0] == "deny"):
		if !cfg.ChatApprovalsEnabled {
			return "Chat approvals are disabled. Enable them with: enable approvals", true
		}
		e, err := h.repo.ResolveEscalationByCode(ctx, c.ID, parts[1], "rejected", msg.SenderID)
		if err != nil {
			return "No pending escalation found for code " + strings.ToUpper(parts[1]) + ".", true
		}
		_ = h.logEvent(ctx, c, e.SessionID, e.RunID, "escalation_rejected", "", map[string]any{"code": e.ApprovalCode, "by": msg.SenderID})
		return "Rejected escalation " + e.ApprovalCode + ".", true
	case body == "disable approvals" || body == "turn off approvals" || body == "disable chat approvals" || body == "turn off chat approvals":
		cfg.ChatApprovalsEnabled = false
		c.Config, _ = json.Marshal(cfg)
		_ = h.repo.UpdateChannel(ctx, &c)
		_ = h.logEvent(ctx, c, "", "", "chat_approvals_disabled", "", map[string]any{"by": msg.SenderID})
		return "Chat approvals are now disabled for this WhatsApp channel.", true
	case body == "enable approvals" || body == "turn on approvals" || body == "enable chat approvals" || body == "turn on chat approvals":
		cfg.ChatApprovalsEnabled = true
		c.Config, _ = json.Marshal(cfg)
		_ = h.repo.UpdateChannel(ctx, &c)
		_ = h.logEvent(ctx, c, "", "", "chat_approvals_enabled", "", map[string]any{"by": msg.SenderID})
		return "Chat approvals are now enabled for this WhatsApp channel.", true
	case body == "list approvals" || body == "pending approvals":
		if !cfg.ChatApprovalsEnabled {
			return "Chat approvals are disabled. Enable them with: enable approvals", true
		}
		items, err := h.repo.ListEscalations(ctx, c.WorkspaceID, c.ID, "pending", 10)
		if err != nil || len(items) == 0 {
			return "No pending escalations.", true
		}
		var b strings.Builder
		b.WriteString("Pending escalations:\n")
		for _, e := range items {
			b.WriteString("* " + e.ApprovalCode + " — " + e.ActionType)
			if e.Recipient != "" {
				b.WriteString(" to " + e.Recipient)
			}
			if e.Reason != "" {
				b.WriteString(": " + e.Reason)
			}
			b.WriteString("\n")
		}
		b.WriteString("Reply approve CODE or reject CODE.")
		return b.String(), true
	}
	return "", false
}

func (h *GatewayHandler) isOwnerSender(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, contact *domain.GatewayContact) bool {
	if contact != nil && contact.Role == "owner" {
		return true
	}
	if msg.FromMe && cfg.SelfChatEnabled {
		return true
	}
	owners, err := h.repo.ListOwnerContacts(ctx, c.ID, defaultString(msg.AccountID, cfg.AccountID))
	if err != nil {
		return false
	}
	senderPhone := normalizeGatewayPhone(msg.SenderPhone)
	senderJID := strings.TrimSpace(msg.SenderID)
	for _, owner := range owners {
		ownerPhone := normalizeGatewayPhone(owner.PhoneNumber)
		if senderPhone != "" && ownerPhone != "" && senderPhone == ownerPhone {
			return true
		}
		if senderJID != "" && owner.WhatsAppJID != "" && senderJID == owner.WhatsAppJID {
			return true
		}
		if senderJID != "" && ownerPhone != "" && senderJID == jidFromPhone(ownerPhone) {
			return true
		}
	}
	return false
}

func (h *GatewayHandler) handleOwnerContactToggle(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, target string, enable bool) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "Tell me which contact to " + startStopWord(enable) + ".", true
	}
	matches, err := h.repo.SearchContacts(ctx, c.WorkspaceID, c.ID, cfg.AccountID, target, 5)
	if err == nil && len(matches) == 1 {
		contact := matches[0]
		if enable && contact.Role == "blocked" {
			contact.Role = "trusted"
		}
		contact.AutoReplyEnabled = enable
		if err := h.repo.UpdateContact(ctx, &contact); err != nil {
			return "I couldn't update " + contact.DisplayName + ".", true
		}
		_ = h.logEvent(ctx, c, "", "", "contact_auto_reply_"+enabledDisabled(enable), "", map[string]any{"contact_id": contact.ID, "by": msg.SenderID})
		return contact.DisplayName + " is now " + enabledDisabled(enable) + " for assistant replies.", true
	}
	if err == nil && len(matches) > 1 {
		var b strings.Builder
		b.WriteString("I found multiple contacts. Use a more specific name:\n")
		for _, contact := range matches {
			b.WriteString("* " + contact.DisplayName)
			if contact.PhoneNumber != "" {
				b.WriteString(" (" + contact.PhoneNumber + ")")
			}
			b.WriteString("\n")
		}
		return b.String(), true
	}
	if !enable {
		return "I couldn't find " + target + " in contacts.", true
	}
	name, phone := splitContactNamePhone(target)
	if phone == "" {
		return "I couldn't find " + target + ". To add from chat, send: start Name +PhoneNumber", true
	}
	if name == "" {
		name = phone
	}
	contact := &domain.GatewayContact{
		WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AccountID: cfg.AccountID,
		DisplayName: name, Alias: aliasFromName(name), PhoneNumber: phone, WhatsAppJID: jidFromPhone(phone),
		Role: "trusted", AutoReplyEnabled: true,
	}
	if err := h.repo.CreateContact(ctx, contact); err != nil {
		return "I couldn't add " + name + " as a contact.", true
	}
	_ = h.logEvent(ctx, c, "", "", "contact_created_by_owner", "", map[string]any{"contact_id": contact.ID, "by": msg.SenderID})
	return name + " has been added and assistant replies are enabled.", true
}

func (h *GatewayHandler) setAssistantEnabled(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, enabled bool) (string, bool) {
	cfg.AssistantEnabled = enabled
	c.Config, _ = json.Marshal(cfg)
	_ = h.repo.UpdateChannel(ctx, &c)
	if enabled {
		_ = h.logEvent(ctx, c, "", "", "assistant_enabled_by_owner", "", map[string]any{"by": msg.SenderID})
		return "Assistant replies are now enabled for this WhatsApp channel.", true
	}
	_ = h.logEvent(ctx, c, "", "", "assistant_disabled_by_owner", "", map[string]any{"by": msg.SenderID})
	return "Assistant replies are now stopped for this WhatsApp channel. Send start assistant to turn them back on.", true
}

func (h *GatewayHandler) setBotMode(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, enabled bool) (string, bool) {
	cfg.BotModeEnabled = enabled
	c.Config, _ = json.Marshal(cfg)
	_ = h.repo.UpdateChannel(ctx, &c)
	if enabled {
		_ = h.logEvent(ctx, c, "", "", "bot_mode_enabled_by_owner", "", map[string]any{"by": msg.SenderID})
		return "Bot mode is now enabled. Unknown WhatsApp senders will be silently approved for this channel.", true
	}
	_ = h.logEvent(ctx, c, "", "", "bot_mode_disabled_by_owner", "", map[string]any{"by": msg.SenderID})
	return "Bot mode is now disabled. Only dashboard contacts or contacts you start from chat will receive assistant replies.", true
}

func (h *GatewayHandler) autoApproveBotSender(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage) *domain.GatewayContact {
	name := defaultString(msg.SenderName, defaultString(msg.SenderPhone, msg.SenderID))
	phone := normalizeGatewayPhone(msg.SenderPhone)
	contact := &domain.GatewayContact{
		WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AccountID: defaultString(msg.AccountID, cfg.AccountID),
		DisplayName: name, Alias: aliasFromName(name), PhoneNumber: phone,
		WhatsAppJID: defaultString(msg.SenderID, jidFromPhone(phone)), Role: "trusted", AutoReplyEnabled: true,
	}
	if err := h.repo.CreateContact(ctx, contact); err != nil {
		existing, _ := h.repo.MatchContact(ctx, c.ID, contact.AccountID, msg.SenderID, msg.SenderPhone)
		return existing
	}
	_ = h.logEvent(ctx, c, "", "", "sender_auto_approved_bot_mode", "", map[string]any{
		"sender_id": msg.SenderID, "sender_phone": msg.SenderPhone, "contact_id": contact.ID,
	})
	return contact
}

func (h *GatewayHandler) dispatchGatewayRun(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage) (runID, sessionID, convID string, err error) {
	if msg.AccountID == "" {
		msg.AccountID = cfg.AccountID
	}
	agentID := c.AgentID
	if msg.Contact != nil && msg.Contact.AgentID != "" {
		agentID = msg.Contact.AgentID
	}
	sessionKey := fmt.Sprintf("agent:%s:%s:%s:%s:%s", agentID, c.ChannelType, msg.AccountID, msg.PeerKind, msg.PeerID)
	convID, err = h.ensureGatewayConversation(ctx, c, sessionKey)
	if err != nil {
		return "", "", "", err
	}
	route, _ := json.Marshal(map[string]any{
		"source": msg.Source, "peer_kind": msg.PeerKind, "peer_id": msg.PeerID,
		"contact_id": contactID(msg.Contact), "contact_alias": contactAlias(msg.Contact), "contact_role": contactRole(msg.Contact),
	})
	session, _, err := h.repo.UpsertSession(ctx, &domain.ChannelSession{
		WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AccountID: msg.AccountID, AgentID: agentID,
		ConversationID: convID, SessionKey: sessionKey, PeerKind: msg.PeerKind, PeerID: msg.PeerID,
		ExternalSenderID: msg.SenderID, ActivationMode: "always", LastRoute: route,
	})
	if err != nil {
		return "", "", "", err
	}
	sessionID = session.ID
	convID = session.ConversationID
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, msg.Body); err != nil {
		return "", "", "", err
	}
	runID = uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,channel_session_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid)`,
		runID, c.WorkspaceID, agentID, convID, c.CreatedBy, msg.Body, sessionID); err != nil {
		return "", "", "", err
	}
	_ = h.logEvent(ctx, c, sessionID, runID, "run_started", "", map[string]any{"input": msg.Body})
	agentRepo := repository.NewAgentRepository(h.pool)
	a, err := agentRepo.Get(ctx, agentID, c.WorkspaceID)
	if err != nil {
		return "", "", "", err
	}
	go h.invoke.executeRun(context.Background(), a, c.WorkspaceID, c.CreatedBy, runID, convID, msg.Body, nil, nil)
	return runID, sessionID, convID, nil
}

func (h *GatewayHandler) ensureGatewayConversation(ctx context.Context, c domain.GatewayChannel, sessionKey string) (string, error) {
	var convID string
	err := h.pool.QueryRow(ctx, `SELECT conversation_id::text FROM channel_sessions WHERE channel_id=$1::uuid AND session_key=$2`, c.ID, sessionKey).Scan(&convID)
	if err == nil {
		return convID, nil
	}
	convID = uuid.NewString()
	_, err = h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
		convID, c.WorkspaceID, c.AgentID, c.CreatedBy, "Gateway: "+c.Name)
	return convID, err
}

func (h *GatewayHandler) deliverWhenComplete(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage, runID, sessionID string) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	deadline := time.After(5 * time.Minute)
	for {
		select {
		case <-deadline:
			_ = h.logEvent(ctx, c, sessionID, runID, "delivery_failed", "", map[string]any{"reason": "timeout"})
			return
		case <-t.C:
			var output, status string
			if err := h.pool.QueryRow(ctx, `SELECT output, status FROM runs WHERE id=$1::uuid`, runID).Scan(&output, &status); err != nil {
				continue
			}
			if status == "running" || status == "pending" || status == "approval_wait" {
				continue
			}
			if status != "success" {
				_ = h.logEvent(ctx, c, sessionID, runID, "delivery_failed", "", map[string]any{"run_status": status})
				return
			}
			h.sendAdapterMessage(ctx, c, cfg, msg.AccountID, msg.PeerKind, msg.PeerID, output, sessionID, runID)
			return
		}
	}
}

func (h *GatewayHandler) sendAdapterMessage(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, accountID, peerKind, peerID, text, sessionID, runID string) {
	_, _ = gatewayservice.NewService(h.pool).SendWhatsApp(ctx, gatewayservice.SendRequest{
		Channel: c, Config: cfg, AccountID: accountID, PeerKind: peerKind, PeerID: peerID, Body: text, SessionID: sessionID, RunID: runID,
	})
}

func (h *GatewayHandler) resolveContact(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, msg inboundMessage) (*domain.GatewayContact, string) {
	contact, err := h.repo.MatchContact(ctx, c.ID, defaultString(msg.AccountID, cfg.AccountID), msg.SenderID, msg.SenderPhone)
	if err != nil || contact == nil {
		return nil, "unmatched"
	}
	if contact.Role == "blocked" || !contact.AutoReplyEnabled {
		return contact, "blocked"
	}
	return contact, "contact_allowed"
}

func (h *GatewayHandler) senderAllowed(cfg domain.GatewayChannelConfig, msg inboundMessage, decision string) bool {
	if msg.Source == "http" {
		return true
	}
	if msg.FromMe && cfg.SelfChatEnabled {
		return true
	}
	if decision == "contact_allowed" {
		return true
	}
	if msg.PeerKind == "group" && cfg.GroupPolicy == "disabled" {
		return false
	}
	policy := cfg.DMPolicy
	if msg.PeerKind == "group" {
		policy = cfg.GroupPolicy
	}
	switch policy {
	case "open":
		return true
	case "allowlist", "pairing":
		for _, s := range cfg.AllowFrom {
			if s == msg.SenderID || s == msg.SenderPhone {
				return true
			}
		}
		for _, s := range cfg.GroupAllowFrom {
			if s == msg.PeerID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (h *GatewayHandler) addSenderToAllowlist(ctx context.Context, channelID, ws, sender string) error {
	c, err := h.repo.GetChannelInWorkspace(ctx, channelID, ws)
	if err != nil {
		return err
	}
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	for _, existing := range cfg.AllowFrom {
		if existing == sender {
			return nil
		}
	}
	cfg.AllowFrom = append(cfg.AllowFrom, sender)
	c.Config, _ = json.Marshal(cfg)
	return h.repo.UpdateChannel(ctx, &c)
}

func (h *GatewayHandler) createTrustedContactFromPairing(ctx context.Context, p domain.GatewayPairingRequest) error {
	phone := normalizeGatewayPhone(p.SenderID)
	contact := &domain.GatewayContact{
		WorkspaceID: p.WorkspaceID, ChannelID: p.ChannelID, AccountID: p.AccountID,
		DisplayName: p.SenderID, Alias: aliasFromName(p.SenderID), PhoneNumber: phone,
		WhatsAppJID: defaultString(p.SenderID, jidFromPhone(phone)), Role: "trusted", AutoReplyEnabled: true,
	}
	if !strings.Contains(contact.WhatsAppJID, "@") {
		contact.WhatsAppJID = jidFromPhone(phone)
	}
	return h.repo.CreateContact(ctx, contact)
}

func (h *GatewayHandler) logEvent(ctx context.Context, c domain.GatewayChannel, sessionID, runID, typ, providerMessageID string, payload any) error {
	b, _ := json.Marshal(payload)
	err := h.repo.CreateEvent(ctx, &domain.GatewayEvent{
		WorkspaceID: c.WorkspaceID, ChannelID: c.ID, SessionID: sessionID, RunID: runID,
		EventType: typ, ProviderMessageID: providerMessageID, Payload: b,
	})
	return err
}

func (h *GatewayHandler) loadWhatsAppChannel(w http.ResponseWriter, r *http.Request) (domain.GatewayChannel, domain.GatewayChannelConfig, bool) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	c, err := h.repo.GetChannelInWorkspace(r.Context(), chi.URLParam(r, "id"), ws)
	if err != nil || c.ChannelType != "whatsapp" {
		errs.Write(w, errs.NotFound("whatsapp gateway channel not found"))
		return domain.GatewayChannel{}, domain.GatewayChannelConfig{}, false
	}
	return c, h.parseGatewayConfig(c.Config, c.ChannelType), true
}

func (h *GatewayHandler) parseGatewayConfig(raw json.RawMessage, channelType string) domain.GatewayChannelConfig {
	cfg := domain.GatewayChannelConfig{}
	_ = json.Unmarshal(raw, &cfg)
	return h.normalizeGatewayConfig(mustJSON(cfg), channelType)
}

func (h *GatewayHandler) normalizeGatewayConfig(raw json.RawMessage, channelType string) domain.GatewayChannelConfig {
	cfg := domain.GatewayChannelConfig{}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.AccountID == "" {
		cfg.AccountID = "default"
	}
	if cfg.AdapterURL == "" && channelType == "whatsapp" {
		cfg.AdapterURL = h.cfg.WhatsAppAdapterURL
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
	if channelType == "whatsapp" && !hasJSONKey(raw, "assistant_enabled") {
		cfg.AssistantEnabled = true
	}
	if channelType == "whatsapp" && !hasJSONKey(raw, "chat_approvals_enabled") {
		cfg.ChatApprovalsEnabled = true
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 50
	}
	return cfg
}

func redactGatewayConfig(raw json.RawMessage) json.RawMessage {
	cfg := domain.GatewayChannelConfig{}
	_ = json.Unmarshal(raw, &cfg)
	b, _ := json.Marshal(cfg)
	return b
}

func hasJSONKey(raw json.RawMessage, key string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func adapterGet(ctx context.Context, base, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	return adapterDo(req)
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

func intParam(r *http.Request, key string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(r.URL.Query().Get(key), "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func pairingCode() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strings.ToUpper(uuid.NewString()[:6])
	}
	return fmt.Sprintf("%02X%02X%02X", b[0], b[1], b[2])
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

var nonDigit = regexp.MustCompile(`\D+`)

func normalizeGatewayPhone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	prefix := ""
	if strings.HasPrefix(s, "+") {
		prefix = "+"
	}
	return prefix + nonDigit.ReplaceAllString(s, "")
}

func jidFromPhone(phone string) string {
	p := strings.TrimPrefix(normalizeGatewayPhone(phone), "+")
	if p == "" {
		return ""
	}
	return p + "@s.whatsapp.net"
}

func aliasFromName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func validGatewayContactRole(role string) bool {
	return role == "owner" || role == "trusted" || role == "blocked"
}

func startStopWord(enable bool) string {
	if enable {
		return "start"
	}
	return "stop"
}

func enabledDisabled(enable bool) string {
	if enable {
		return "enabled"
	}
	return "disabled"
}

func splitContactNamePhone(s string) (string, string) {
	if name, phone := extractNamePhone(s); phone != "" {
		return name, phone
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	phoneIndex := -1
	for i, field := range fields {
		normalized := normalizeGatewayPhone(field)
		if looksLikeGatewayPhone(normalized) {
			phoneIndex = i
			fields[i] = normalized
			break
		}
	}
	if phoneIndex < 0 {
		return strings.TrimSpace(s), ""
	}
	phone := fields[phoneIndex]
	nameFields := append([]string{}, fields[:phoneIndex]...)
	nameFields = append(nameFields, fields[phoneIndex+1:]...)
	return strings.TrimSpace(strings.Join(nameFields, " ")), phone
}

func looksLikeGatewayPhone(s string) bool {
	digits := strings.TrimPrefix(normalizeGatewayPhone(s), "+")
	return len(digits) >= 8
}

func parseOwnerNaturalCommand(raw string) (intent, target string, ok bool) {
	body := strings.ToLower(strings.TrimSpace(raw))
	if body == "" {
		return "", "", false
	}
	switch {
	case containsAny(body, "enable bot mode", "turn on bot mode", "mark channel as bot", "mark this channel as bot", "make this channel a bot"):
		return "enable_bot_mode", "", true
	case containsAny(body, "disable bot mode", "turn off bot mode", "unmark channel as bot", "unmark this channel as bot"):
		return "disable_bot_mode", "", true
	case containsAny(body, "start assistant", "resume assistant", "turn on assistant", "start replies", "resume replies"):
		return "start_assistant", "", true
	case containsAny(body, "stop assistant", "pause assistant", "turn off assistant", "stop replies", "pause replies"):
		return "stop_assistant", "", true
	}
	if containsAny(body, "stop replying to ", "pause replying to ", "disable replies for ", "turn off replies for ", "stop the agent for ", "pause the agent for ") {
		return "stop_contact", cleanOwnerCommandTarget(raw, false), true
	}
	if containsAny(body, "start replying to ", "auto reply to ", "auto-reply to ", "allow ", "enable replies for ", "turn on replies for ", "let ") &&
		containsAny(body, "agent", "assistant", "reply", "talk", "message", "whatsapp") {
		return "start_contact", cleanOwnerCommandTarget(raw, true), true
	}
	if _, phone := extractNamePhone(raw); phone != "" && containsAny(body, "start ", "add ", "allow ", "enable ") {
		return "start_contact", cleanOwnerCommandTarget(raw, true), true
	}
	return "", "", false
}

func cleanOwnerCommandTarget(raw string, enable bool) string {
	s := strings.TrimSpace(raw)
	replacements := []string{
		"start replying to ", "", "auto reply to ", "", "auto-reply to ", "", "allow ", "", "enable replies for ", "", "turn on replies for ", "",
		"let ", "", " talk to the agent", "", " talk to assistant", "", " talk to the assistant", "",
		" to talk to the agent", "", " to talk to assistant", "", " to talk to the assistant", "",
		" access the agent", "", " use the agent", "", " message the agent", "", " her number is ", " ",
		" his number is ", " ", " their number is ", " ", " number is ", " ", " on whatsapp", "",
	}
	if !enable {
		replacements = []string{
			"stop replying to ", "", "pause replying to ", "", "disable replies for ", "", "turn off replies for ", "",
			"stop the agent for ", "", "pause the agent for ", "",
		}
	}
	lower := strings.ToLower(s)
	for i := 0; i < len(replacements); i += 2 {
		from, to := replacements[i], replacements[i+1]
		if strings.Contains(lower, from) {
			idx := strings.Index(lower, from)
			s = s[:idx] + to + s[idx+len(from):]
			lower = strings.ToLower(s)
		}
	}
	return strings.Trim(strings.TrimSpace(s), ".,;:")
}

var phoneInText = regexp.MustCompile(`\+?\d[\d\s().-]{7,}\d`)

func extractNamePhone(raw string) (string, string) {
	match := phoneInText.FindString(raw)
	if match == "" {
		return strings.TrimSpace(raw), ""
	}
	phone := normalizeGatewayPhone(match)
	if !looksLikeGatewayPhone(phone) {
		return strings.TrimSpace(raw), ""
	}
	name := strings.TrimSpace(strings.Replace(raw, match, "", 1))
	name = strings.Trim(strings.TrimSpace(name), ".,;:-")
	return name, phone
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func contactID(c *domain.GatewayContact) string {
	if c == nil {
		return ""
	}
	return c.ID
}

func contactAlias(c *domain.GatewayContact) string {
	if c == nil {
		return ""
	}
	return c.Alias
}

func contactRole(c *domain.GatewayContact) string {
	if c == nil {
		return ""
	}
	return c.Role
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

var whatsappManagedSkillNames = []string{
	"WhatsApp Formatter",
	"WhatsApp Messaging Operator",
	"WhatsApp Contact Resolver",
	"WhatsApp Raw Number Sender",
	"WhatsApp Safety & Consent",
	"WhatsApp Identity Verification",
	"WhatsApp Group Chat Operator",
	"WhatsApp Conversation Memory",
	"WhatsApp Follow-up Scheduler",
	"WhatsApp Delivery Recovery",
	"WhatsApp Media Handler",
	"WhatsApp Link Summarizer",
	"WhatsApp Task Intake",
	"WhatsApp Owner Escalation",
	"WhatsApp Personal Assistant",
}

var whatsappToolNames = []string{
	"whatsapp_search_contacts",
	"whatsapp_send_message",
	"whatsapp_get_current_context",
	"whatsapp_list_recent_messages",
	"whatsapp_create_reminder",
	"whatsapp_list_reminders",
	"whatsapp_complete_reminder",
	"whatsapp_summarize_link",
	"whatsapp_request_owner_approval",
	"whatsapp_send_media_status",
}

func AttachWhatsAppCapabilities(ctx context.Context, pool *pgxpool.Pool, agentID string) error {
	for i, name := range whatsappManagedSkillNames {
		if _, err := pool.Exec(ctx, `
			INSERT INTO agent_skills(agent_id, skill_id, enabled, order_index)
			SELECT $1::uuid, id, true, $3
			FROM skills
			WHERE workspace_id IS NULL AND name=$2
			ON CONFLICT(agent_id, skill_id) DO UPDATE SET enabled=true`,
			agentID, name, i); err != nil {
			return err
		}
	}
	for _, name := range whatsappToolNames {
		if _, err := pool.Exec(ctx, `
			INSERT INTO agent_tools(agent_id, tool_id, enabled)
			SELECT $1::uuid, id, true
			FROM tools
			WHERE workspace_id IS NULL AND name=$2
			ON CONFLICT(agent_id, tool_id) DO UPDATE SET enabled=true`,
			agentID, name); err != nil {
			return err
		}
	}
	return nil
}
