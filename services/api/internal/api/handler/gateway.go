package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
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
	// For WhatsApp channels: log out the adapter before deleting so credentials
	// are wiped from the DB and the socket is closed cleanly.
	if ch, err := h.repo.GetChannelInWorkspace(r.Context(), id, ws); err == nil && ch.ChannelType == "whatsapp" {
		cfg := h.parseGatewayConfig(ch.Config, ch.ChannelType)
		// Best-effort — ignore errors; channel deletion proceeds regardless.
		adapterPost(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/logout", nil) //nolint:errcheck
	}
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
	q := r.URL.Query().Get("q")
	page := intParam(r, "page", 1)
	perPage := intParam(r, "per_page", 20)
	list, total, err := h.repo.ListContacts(r.Context(), ws, r.URL.Query().Get("channel_id"), q, page, perPage)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list gateway contacts"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"data":     list,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
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

func (h *GatewayHandler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var body domain.GatewayReminder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.ChannelID == "" || body.Title == "" || body.DueAt == nil {
		errs.Write(w, errs.BadRequest("channel_id, title, and due_at are required"))
		return
	}
	// Resolve account_id + peer from contact when available so the dispatcher can deliver it.
	if body.AccountID == "" || body.ContactID != "" {
		if body.ContactID != "" {
			if _, err := h.repo.GetContact(r.Context(), body.ContactID, ws); err != nil {
				errs.Write(w, errs.BadRequest("contact not found"))
				return
			}
		}
		if ch, err := h.repo.GetChannel(r.Context(), body.ChannelID); err == nil {
			cfg := gatewayservice.ParseConfig(ch.Config, h.cfg.WhatsAppAdapterURL)
			body.AccountID = cfg.AccountID
		}
	}
	body.WorkspaceID = ws
	if body.Message == "" {
		body.Message = body.Title
	}
	if err := h.repo.CreateReminder(r.Context(), &body); err != nil {
		errs.Write(w, errs.Internal("failed to create reminder"))
		return
	}
	errs.WriteJSON(w, http.StatusCreated, body)
}

func (h *GatewayHandler) UpdateReminder(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var body domain.GatewayReminder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.Title == "" || body.DueAt == nil {
		errs.Write(w, errs.BadRequest("title and due_at are required"))
		return
	}
	if body.Message == "" {
		body.Message = body.Title
	}
	m, err := h.repo.UpdateReminder(r.Context(), chi.URLParam(r, "id"), ws, &body)
	if err != nil {
		errs.Write(w, errs.NotFound("reminder not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, m)
}

func (h *GatewayHandler) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	if _, err := h.repo.UpdateReminderStatus(r.Context(), chi.URLParam(r, "id"), ws, "cancelled"); err != nil {
		errs.Write(w, errs.NotFound("reminder not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (h *GatewayHandler) AdapterSyncLIDs(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	body, err := adapterPost(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/sync-lids", nil)
	if err != nil {
		errs.Write(w, errs.BadRequest("lid sync failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

func (h *GatewayHandler) AdapterSyncContacts(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	body, err := adapterGet(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/contacts")
	if err != nil {
		errs.Write(w, errs.BadRequest("adapter contacts failed: "+err.Error()))
		return
	}
	var resp struct {
		Contacts []struct {
			Phone string `json:"phone"`
			JID   string `json:"jid"`
			LID   string `json:"lid"`
			Name  string `json:"name"`
		} `json:"contacts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		errs.Write(w, errs.BadRequest("invalid adapter response"))
		return
	}
	var created, updated int
	for _, ac := range resp.Contacts {
		phone := normalizeGatewayPhone(ac.Phone)
		if phone == "" {
			continue
		}
		existing, _ := h.repo.MatchContact(r.Context(), c.ID, cfg.AccountID, ac.JID, phone)
		if existing == nil {
			name := defaultString(ac.Name, phone)
			gc := &domain.GatewayContact{
				WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AccountID: cfg.AccountID,
				DisplayName: name, Alias: aliasFromName(name),
				PhoneNumber: phone, WhatsAppJID: ac.JID, WhatsAppLID: ac.LID,
				Role: "trusted", AutoReplyEnabled: false,
			}
			if err := h.repo.CreateContact(r.Context(), gc); err == nil {
				created++
			}
		} else if existing.WhatsAppLID == "" && ac.LID != "" {
			_, _ = h.pool.Exec(r.Context(),
				`UPDATE gateway_contacts SET whatsapp_lid=$1, updated_at=NOW() WHERE id=$2::uuid`,
				ac.LID, existing.ID)
			updated++
		}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated})
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
		"browser_name":      cfg.BrowserName,
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
	if _, err := adapterPost(context.Background(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/start", map[string]any{
		"contact_phones": phones,
	}); err != nil {
		slog.Warn("gateway contact-phone sync to adapter failed", "channel_id", channelID, "account", cfg.AccountID, "error", err)
	}
}

func (h *GatewayHandler) syncAdapterConfig(ctx context.Context, c domain.GatewayChannel) {
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	if _, err := adapterPost(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/config", map[string]any{
		"channel_id":        c.ID,
		"callback_url":      strings.TrimRight(h.cfg.PublicAPIURL, "/") + "/gateway/whatsapp/" + c.ID,
		"self_chat_enabled": cfg.SelfChatEnabled,
	}); err != nil {
		slog.Warn("gateway adapter config sync failed", "channel_id", c.ID, "account", cfg.AccountID, "error", err)
	}
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
			"browser_name":      cfg.BrowserName,
		})
		if err != nil {
			slog.Warn("adapter startup sync: login/start failed", "channel", c.ID, "error", err)
		} else {
			slog.Info("adapter startup sync: channel reconnected", "channel", c.ID, "account", cfg.AccountID, "contact_phones", len(phones))
		}
		// Push config via /config so selfChatEnabled is applied even when the socket was
		// already connected and startAccount returned early without updating state.
		h.syncAdapterConfig(ctx, c)
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

func (h *GatewayHandler) AdapterPairingCode(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := h.loadWhatsAppChannel(w, r)
	if !ok {
		return
	}
	var body struct {
		Phone string `json:"phone"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Phone) == "" {
		errs.Write(w, errs.BadRequest("phone number is required"))
		return
	}
	// Strip non-digits; caller may pass "+91..." or "91..."
	var digits strings.Builder
	for _, ch := range body.Phone {
		if ch >= '0' && ch <= '9' {
			digits.WriteRune(ch)
		}
	}
	result, err := adapterPost(r.Context(), cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/login/pairing-code", map[string]any{
		"phone": digits.String(),
	})
	if err != nil {
		errs.Write(w, errs.BadRequest("pairing code request failed: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(result) //nolint:errcheck
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
	// Self-chat echo guard: replies this channel just sent come back as
	// from_me inbound events. The adapter filters them by message id, but
	// that filter races the send ack and is lost on adapter restart — an
	// echo that slips through gets treated as an owner message and
	// dispatched to the agent, which then replies to the gateway's own
	// reply (the "Which Rajat?" loop).
	if msg.FromMe {
		if recent, err := h.repo.HasRecentOutboundBody(ctx, c.ID, msg.PeerID, msg.Body, 2*time.Minute); err != nil {
			slog.Warn("gateway echo check failed", "channel_id", c.ID, "peer_id", msg.PeerID, "error", err)
		} else if recent {
			_ = h.logEvent(ctx, c, "", "", "self_echo_dropped", msg.ProviderMessageID, map[string]any{"peer_id": msg.PeerID})
			return false, ""
		}
	}
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
	case body == "sync" || body == "sync contacts" || body == "!sync":
		accountID := defaultString(msg.AccountID, cfg.AccountID)
		result, err := adapterPost(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(accountID)+"/sync-lids", nil)
		if err != nil {
			return "Contact sync failed: " + err.Error(), true
		}
		var syncRes struct {
			Synced int `json:"synced"`
		}
		_ = json.Unmarshal(result, &syncRes)
		return fmt.Sprintf("Synced %d contact LID mappings to the database.", syncRes.Synced), true
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
		b.WriteString("I found multiple contacts:\n")
		for _, contact := range matches {
			b.WriteString("* " + contact.DisplayName)
			if contact.PhoneNumber != "" {
				b.WriteString(" (" + contact.PhoneNumber + ")")
			}
			b.WriteString("\n")
		}
		example := matches[0].DisplayName
		if matches[0].PhoneNumber != "" {
			example = matches[0].PhoneNumber
		}
		b.WriteString("Reply with the phone number to pick one, e.g.: " + startStopWord(enable) + " " + example)
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
	// Canonicalise the peer for the session key: WhatsApp delivers the same
	// direct chat under both phone JIDs and @lid JIDs (the adapter's LID
	// resolution can miss when its map is cold), and a flapping peer id
	// forks the session — the agent silently starts a fresh conversation
	// mid-dialogue and loses all context. The matched contact's stored JID
	// is stable across both forms (contacts match by phone AND lid).
	sessionPeer := msg.PeerID
	if msg.PeerKind == "direct" && msg.Contact != nil && msg.Contact.WhatsAppJID != "" {
		sessionPeer = msg.Contact.WhatsAppJID
	}
	sessionKey := fmt.Sprintf("agent:%s:%s:%s:%s:%s", agentID, c.ChannelType, msg.AccountID, msg.PeerKind, sessionPeer)

	// If a run is waiting for user input on this session, deliver the reply to it
	// instead of creating a new run.
	var waitingRunID string
	_ = h.pool.QueryRow(ctx,
		`SELECT r.id::text FROM runs r
		 JOIN channel_sessions cs ON cs.id = r.channel_session_id
		 WHERE cs.channel_id=$1::uuid AND cs.session_key=$2 AND r.status='user_input_wait'
		 ORDER BY r.created_at DESC LIMIT 1`,
		c.ID, sessionKey).Scan(&waitingRunID)
	if waitingRunID != "" && SendUserInput(waitingRunID, msg.Body) {
		return "", "", "", nil
	}

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

	var sendMsg func(ctx context.Context, text string) error
	if c.ChannelType == "whatsapp" {
		var (
			lastSent time.Time
			mu       sync.Mutex
			ch       = c
			chCfg    = cfg
			peerKind = msg.PeerKind
			peerID   = msg.PeerID
			sessID   = sessionID
		)
		sendMsg = func(ctx context.Context, text string) error {
			mu.Lock()
			if time.Since(lastSent) < 3*time.Second {
				mu.Unlock()
				return nil
			}
			lastSent = time.Now()
			mu.Unlock()
			h.sendAdapterMessage(ctx, ch, chCfg, chCfg.AccountID, peerKind, peerID, text, sessID, runID)
			return nil
		}
	}

	go h.invoke.executeRun(context.Background(), a, c.WorkspaceID, c.CreatedBy, runID, convID, msg.Body, nil, nil, invokeOpts{SendMessage: sendMsg})
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
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	deadline := time.After(5 * time.Minute)
	for {
		select {
		case <-deadline:
			_ = h.logEvent(ctx, c, sessionID, runID, "delivery_failed", "", map[string]any{"reason": "timeout"})
			h.sendAdapterMessage(ctx, c, cfg, msg.AccountID, msg.PeerKind, msg.PeerID, "Sorry, the request timed out. Please try again.", sessionID, runID)
			return
		case <-t.C:
			var output, status, errMsg string
			if err := h.pool.QueryRow(ctx, `SELECT output, status, error_message FROM runs WHERE id=$1::uuid`, runID).Scan(&output, &status, &errMsg); err != nil {
				continue
			}
			if status == "running" || status == "pending" || status == "approval_wait" || status == "user_input_wait" {
				continue
			}
			if status != "success" {
				_ = h.logEvent(ctx, c, sessionID, runID, "delivery_failed", "", map[string]any{"run_status": status, "error": errMsg})
				// Send a user-facing error reply. If the agent produced partial output before
				// failing, use that; otherwise fall back to a generic message.
				reply := strings.TrimSpace(output)
				if reply == "" {
					reply = "Sorry, I ran into an error and couldn't complete your request. Please try again."
				}
				h.sendAdapterMessage(ctx, c, cfg, msg.AccountID, msg.PeerKind, msg.PeerID, reply, sessionID, runID)
				return
			}
			crossContactSend, err := h.repo.HasSentAgentDirectToOtherPeer(ctx, c.WorkspaceID, c.ID, runID, msg.PeerID)
			if err != nil {
				slog.Warn("failed to check direct WhatsApp sends", "run_id", runID, "error", err)
			}
			h.sendAdapterMessage(ctx, c, cfg, msg.AccountID, msg.PeerKind, msg.PeerID, completionReply(output, crossContactSend), sessionID, runID)
			return
		}
	}
}

func completionReply(output string, sentToOtherPeer bool) string {
	if sentToOtherPeer {
		return "Message delivered."
	}
	return output
}

func (h *GatewayHandler) sendAdapterMessage(ctx context.Context, c domain.GatewayChannel, cfg domain.GatewayChannelConfig, accountID, peerKind, peerID, text, sessionID, runID string) {
	if _, err := gatewayservice.NewService(h.pool).SendWhatsApp(ctx, gatewayservice.SendRequest{
		Channel: c, Config: cfg, AccountID: accountID, PeerKind: peerKind, PeerID: peerID, Body: text, SessionID: sessionID, RunID: runID,
	}); err != nil {
		// SendWhatsApp already persisted the failed outbound row and the
		// delivery_failed event; this is the operator-facing signal in the
		// server log — previously the error was discarded entirely and an
		// agent reply could vanish without any trace outside the Outbox tab.
		slog.Error("gateway reply delivery failed",
			"channel", c.Name, "channel_id", c.ID, "peer_id", peerID, "run_id", runID, "error", err)
	}
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

// StartConnectionWatchdog checks every 2 min whether any WhatsApp connection is stale
// (connected but no messages received within 90 s of connecting) and reconnects it.
func (h *GatewayHandler) StartConnectionWatchdog(ctx context.Context) {
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.checkAndReconnectStaleConnections(ctx)
		}
	}
}

func (h *GatewayHandler) checkAndReconnectStaleConnections(ctx context.Context) {
	channels, err := h.repo.ListAllActiveWhatsAppChannels(ctx)
	if err != nil {
		return
	}
	for _, c := range channels {
		cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
		statusBody, err := adapterGet(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/status")
		if err != nil {
			continue
		}
		var st struct {
			Status        string `json:"status"`
			ConnectedAt   int64  `json:"connected_at"`
			LastMessageAt int64  `json:"last_message_at"`
		}
		if err := json.Unmarshal(statusBody, &st); err != nil {
			continue
		}
		if st.Status != "connected" {
			continue
		}
		// If connected but no message received within 90 s of connecting, the socket is stale.
		now := time.Now().UnixMilli()
		connectedFor := now - st.ConnectedAt
		if st.ConnectedAt > 0 && st.LastMessageAt == 0 && connectedFor > 90_000 {
			slog.Warn("whatsapp connection stale, reconnecting", "channel", c.ID, "account", cfg.AccountID, "connected_for_ms", connectedFor)
			if _, err := adapterPost(ctx, cfg.AdapterURL, "/accounts/"+url.PathEscape(cfg.AccountID)+"/reconnect", nil); err != nil {
				slog.Error("whatsapp stale reconnect failed", "channel", c.ID, "account", cfg.AccountID, "error", err)
			}
		}
	}
}

// StartReminderDispatcher ticks every 30 s and sends any WhatsApp reminders that are past due.
func (h *GatewayHandler) StartReminderDispatcher(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.fireReminders(ctx)
		}
	}
}

func (h *GatewayHandler) fireReminders(ctx context.Context) {
	due, err := h.repo.FetchDueReminders(ctx)
	if err != nil {
		slog.Error("reminder dispatcher: fetch failed", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	svc := gatewayservice.NewService(h.pool)
	for _, rem := range due {
		if rem.PeerID == "" {
			slog.Warn("reminder has no peer_id, skipping", "reminder_id", rem.ID, "title", rem.Title)
			_, _ = h.repo.UpdateReminderStatus(ctx, rem.ID, rem.WorkspaceID, "cancelled")
			continue
		}
		ch, err := h.repo.GetChannel(ctx, rem.ChannelID)
		if err != nil {
			slog.Warn("reminder dispatcher: channel not found", "reminder_id", rem.ID, "channel_id", rem.ChannelID)
			continue
		}
		if !ch.IsActive {
			continue
		}
		if ch.ChannelType != "whatsapp" {
			continue
		}
		cfg := gatewayservice.ParseConfig(ch.Config, h.cfg.WhatsAppAdapterURL)
		accountID := rem.AccountID
		if accountID == "" {
			accountID = cfg.AccountID
		}
		msg := rem.Message
		if msg == "" {
			msg = rem.Title
		}
		if rem.UseAgent {
			var contact *domain.GatewayContact
			if rem.ContactID != "" {
				if c, err := h.repo.GetContact(ctx, rem.ContactID, rem.WorkspaceID); err == nil {
					contact = &c
				}
			}
			synthetic := inboundMessage{
				AccountID: accountID,
				Body:      msg,
				PeerKind:  rem.PeerKind,
				PeerID:    rem.PeerID,
				Source:    "reminder",
				Contact:   contact,
			}
			runID, sessionID, _, err := h.dispatchGatewayRun(ctx, ch, cfg, synthetic)
			if err != nil {
				slog.Warn("reminder agent dispatch failed", "reminder_id", rem.ID, "title", rem.Title, "error", err)
				_, _ = h.repo.UpdateReminderStatus(ctx, rem.ID, rem.WorkspaceID, "failed")
				continue
			}
			go h.deliverWhenComplete(context.Background(), ch, cfg, synthetic, runID, sessionID)
		} else {
			_, sendErr := svc.SendWhatsApp(ctx, gatewayservice.SendRequest{
				Channel: ch, Config: cfg, AccountID: accountID,
				PeerKind: rem.PeerKind, PeerID: rem.PeerID, Body: msg,
			})
			if sendErr != nil {
				slog.Warn("reminder send failed", "reminder_id", rem.ID, "title", rem.Title, "error", sendErr)
				_, _ = h.repo.UpdateReminderStatus(ctx, rem.ID, rem.WorkspaceID, "failed")
				continue
			}
		}
		_, _ = h.repo.UpdateReminderStatus(ctx, rem.ID, rem.WorkspaceID, "completed")
		slog.Info("reminder fired", "reminder_id", rem.ID, "title", rem.Title, "peer_id", rem.PeerID)
	}
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
	// Contact-level enable/disable is matched against a normalised copy so
	// the spelling variants owners actually type — "auto-reply", "auto
	// replies", "autoreply" — and both to/for phrasings all hit the same
	// canonical patterns ("Enable Auto replies for Rajat" previously matched
	// nothing and fell through to the agent).
	nb := normalizeReplyPhrase(body)
	// Disable variants must be checked BEFORE the enable patterns: "disable
	// auto reply to X" contains "auto reply to " and previously matched the
	// enable branch, silently inverting the owner's intent.
	if containsAny(nb, "stop replying to ", "pause replying to ", "disable reply for ", "disable reply to ", "turn off reply for ", "turn off reply to ", "stop the agent for ", "pause the agent for ",
		"disable auto reply to ", "disable auto reply for ", "turn off auto reply to ", "turn off auto reply for ", "stop auto reply to ", "stop auto reply for ") {
		return "stop_contact", cleanOwnerCommandTarget(raw, false), true
	}
	if containsAny(nb, "start replying to ", "auto reply to ", "auto reply for ", "allow ", "enable reply for ", "enable reply to ", "turn on reply for ", "turn on reply to ", "let ") &&
		containsAny(nb, "agent", "assistant", "reply", "talk", "message", "whatsapp") {
		return "start_contact", cleanOwnerCommandTarget(raw, true), true
	}
	if _, phone := extractNamePhone(raw); phone != "" && containsAny(body, "start ", "add ", "allow ", "enable ") {
		return "start_contact", cleanOwnerCommandTarget(raw, true), true
	}
	return "", "", false
}

// normalizeReplyPhrase canonicalises the reply-phrase variants owners type —
// hyphenated "auto-reply", joined "autoreply", and plural "replies" — so the
// command patterns only need the "auto reply"/"reply" forms. Two passes: the
// hyphen/joined rewrite first, so "auto-replies" becomes "auto replies" before
// the plural collapses to "reply".
func normalizeReplyPhrase(body string) string {
	s := strings.NewReplacer("auto-repl", "auto repl", "autorepl", "auto repl").Replace(body)
	return strings.ReplaceAll(s, "replies", "reply")
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
			"disable auto reply to ", "", "disable auto-reply to ", "", "turn off auto reply to ", "", "turn off auto-reply to ", "",
			"stop auto reply to ", "", "stop auto-reply to ", "", "disable auto reply for ", "", "disable auto-reply for ", "",
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
	s = strings.Trim(strings.TrimSpace(s), ".,;:")
	// Strip leading command verbs the phrase patterns didn't consume —
	// "Enable auto reply to Rajat" must yield target "Rajat", not
	// "Enable Rajat" (which matches no contact and produces a baffling
	// "I couldn't find Enable Rajat" reply).
	for {
		stripped := false
		lower = strings.ToLower(s)
		for _, verb := range []string{"please ", "enable ", "disable ", "turn on ", "turn off ", "start ", "stop ",
			"auto replies for ", "auto replies to ", "auto-replies for ", "auto-replies to ", "autoreplies for ", "autoreplies to ",
			"auto reply to ", "auto-reply to ", "auto reply for ", "auto-reply for ", "autoreply for ", "autoreply to ",
			"replies for ", "replies to ", "reply to ", "reply for "} {
			if strings.HasPrefix(lower, verb) {
				s = strings.TrimSpace(s[len(verb):])
				stripped = true
				break
			}
		}
		if !stripped {
			break
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
	"WhatsApp Message Scheduler",
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
	"whatsapp_schedule_message",
}

// ─── Scheduled Messages ────────────────────────────────────────────────────

func (h *GatewayHandler) ListScheduledMessages(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	list, err := h.repo.ListScheduledMessages(r.Context(), ws, r.URL.Query().Get("channel_id"), r.URL.Query().Get("status"), intParam(r, "limit", 100))
	if err != nil {
		errs.Write(w, errs.Internal("failed to list scheduled messages"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *GatewayHandler) CreateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	var body domain.ScheduledMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.ChannelID == "" || body.Message == "" || body.SendAt.IsZero() {
		errs.Write(w, errs.BadRequest("channel_id, message, and send_at are required"))
		return
	}
	// Resolve peer_id from contact if not provided.
	if body.PeerID == "" && body.ContactID != "" {
		c, err := h.repo.GetContact(r.Context(), body.ContactID, ws)
		if err != nil {
			errs.Write(w, errs.BadRequest("contact not found"))
			return
		}
		body.PeerID = c.WhatsAppJID
		if body.PeerID == "" {
			body.PeerID = c.PhoneNumber
		}
	}
	if body.PeerID == "" {
		errs.Write(w, errs.BadRequest("peer_id or contact_id is required"))
		return
	}
	if body.PeerKind == "" {
		body.PeerKind = "direct"
	}
	body.WorkspaceID = ws
	body.CreatedBy = uid
	if body.AccountID == "" {
		ch, err := h.repo.GetChannel(r.Context(), body.ChannelID)
		if err == nil {
			cfg := gatewayservice.ParseConfig(ch.Config, h.cfg.WhatsAppAdapterURL)
			body.AccountID = cfg.AccountID
		}
	}
	if err := h.repo.CreateScheduledMessage(r.Context(), &body); err != nil {
		errs.Write(w, errs.Internal("failed to create scheduled message"))
		return
	}
	errs.WriteJSON(w, http.StatusCreated, body)
}

func (h *GatewayHandler) GetScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	m, err := h.repo.GetScheduledMessage(r.Context(), chi.URLParam(r, "id"), ws)
	if err != nil {
		errs.Write(w, errs.NotFound("scheduled message not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, m)
}

func (h *GatewayHandler) UpdateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var body domain.ScheduledMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if body.Message == "" || body.SendAt.IsZero() {
		errs.Write(w, errs.BadRequest("message and send_at are required"))
		return
	}
	// Re-resolve peer_id from contact if a contact is set (contact may have changed).
	if body.ContactID != "" {
		c, err := h.repo.GetContact(r.Context(), body.ContactID, ws)
		if err != nil {
			errs.Write(w, errs.BadRequest("contact not found"))
			return
		}
		body.PeerID = c.WhatsAppJID
		if body.PeerID == "" {
			body.PeerID = c.PhoneNumber
		}
	}
	if body.PeerID == "" {
		errs.Write(w, errs.BadRequest("peer_id or contact_id is required"))
		return
	}
	if body.PeerKind == "" {
		body.PeerKind = "direct"
	}
	m, err := h.repo.UpdateScheduledMessage(r.Context(), chi.URLParam(r, "id"), ws, &body)
	if err != nil {
		errs.Write(w, errs.NotFound("scheduled message not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, m)
}

func (h *GatewayHandler) DeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	if err := h.repo.UpdateScheduledMessageStatus(r.Context(), chi.URLParam(r, "id"), ws, "cancelled", ""); err != nil {
		errs.Write(w, errs.NotFound("scheduled message not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nextScheduledFireAt computes the next fire time for a recurring scheduled message.
// Returns (zero, false) when the series is complete.
func nextScheduledFireAt(rule json.RawMessage, from time.Time, count int) (time.Time, bool) {
	var r struct {
		Frequency      string     `json:"frequency"`
		Interval       int        `json:"interval"`
		EndAt          *time.Time `json:"end_at"`
		MaxOccurrences int        `json:"max_occurrences"`
	}
	if err := json.Unmarshal(rule, &r); err != nil || r.Frequency == "" {
		return time.Time{}, false
	}
	if r.Interval <= 0 {
		r.Interval = 1
	}
	if r.MaxOccurrences > 0 && count >= r.MaxOccurrences {
		return time.Time{}, false
	}
	var next time.Time
	switch r.Frequency {
	case "daily":
		next = from.AddDate(0, 0, r.Interval)
	case "weekly":
		next = from.AddDate(0, 0, 7*r.Interval)
	case "monthly":
		next = from.AddDate(0, r.Interval, 0)
	case "yearly":
		next = from.AddDate(r.Interval, 0, 0)
	case "weekdays":
		next = from.AddDate(0, 0, 1)
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
	default:
		return time.Time{}, false
	}
	if r.EndAt != nil && next.After(*r.EndAt) {
		return time.Time{}, false
	}
	return next, true
}

// StartScheduledMessageDispatcher ticks every 30 s and delivers scheduled messages that are past due.
func (h *GatewayHandler) StartScheduledMessageDispatcher(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.fireScheduledMessages(ctx)
		}
	}
}

func (h *GatewayHandler) fireScheduledMessages(ctx context.Context) {
	due, err := h.repo.FetchDueScheduledMessages(ctx)
	if err != nil {
		slog.Error("scheduled message dispatcher: fetch failed", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	svc := gatewayservice.NewService(h.pool)
	for _, msg := range due {
		ch, err := h.repo.GetChannel(ctx, msg.ChannelID)
		if err != nil {
			slog.Warn("scheduled message dispatcher: channel not found", "msg_id", msg.ID, "channel_id", msg.ChannelID)
			continue
		}
		if !ch.IsActive {
			continue
		}
		cfg := gatewayservice.ParseConfig(ch.Config, h.cfg.WhatsAppAdapterURL)
		accountID := msg.AccountID
		if accountID == "" {
			accountID = cfg.AccountID
		}
		if msg.UseAgent {
			var contact *domain.GatewayContact
			if msg.ContactID != "" {
				if c, err := h.repo.GetContact(ctx, msg.ContactID, msg.WorkspaceID); err == nil {
					contact = &c
				}
			}
			synthetic := inboundMessage{
				AccountID: accountID,
				Body:      msg.Message,
				PeerKind:  msg.PeerKind,
				PeerID:    msg.PeerID,
				Source:    "scheduled",
				Contact:   contact,
			}
			runID, sessionID, _, err := h.dispatchGatewayRun(ctx, ch, cfg, synthetic)
			if err != nil {
				slog.Warn("scheduled message agent dispatch failed", "msg_id", msg.ID, "error", err)
				_ = h.repo.UpdateScheduledMessageStatus(ctx, msg.ID, msg.WorkspaceID, "failed", err.Error())
				continue
			}
			go h.deliverWhenComplete(context.Background(), ch, cfg, synthetic, runID, sessionID)
		} else {
			_, sendErr := svc.SendWhatsApp(ctx, gatewayservice.SendRequest{
				Channel: ch, Config: cfg, AccountID: accountID,
				PeerKind: msg.PeerKind, PeerID: msg.PeerID, Body: msg.Message,
			})
			if sendErr != nil {
				slog.Warn("scheduled message send failed", "msg_id", msg.ID, "peer", msg.PeerID, "error", sendErr)
				_ = h.repo.UpdateScheduledMessageStatus(ctx, msg.ID, msg.WorkspaceID, "failed", sendErr.Error())
				continue
			}
		}
		if len(msg.RecurrenceRule) > 2 {
			// Base the next fire on the scheduled send time (not time.Now()) so the
			// time-of-day the user picked is preserved across occurrences instead of
			// drifting by the dispatcher's tick latency. If the dispatcher was down and
			// the series fell behind, advance until the next fire is in the future so we
			// don't emit a burst of catch-up sends.
			next, ok := nextScheduledFireAt(msg.RecurrenceRule, msg.SendAt, msg.OccurrenceCount+1)
			for ok && !next.After(time.Now()) {
				next, ok = nextScheduledFireAt(msg.RecurrenceRule, next, msg.OccurrenceCount+1)
			}
			if ok {
				_ = h.repo.RescheduleMessage(ctx, msg.ID, msg.WorkspaceID, next, msg.OccurrenceCount+1)
				slog.Info("scheduled message rescheduled", "msg_id", msg.ID, "next_send_at", next)
				continue
			}
		}
		_ = h.repo.UpdateScheduledMessageStatus(ctx, msg.ID, msg.WorkspaceID, "sent", "")
		slog.Info("scheduled message sent", "msg_id", msg.ID, "peer", msg.PeerID)
	}
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
