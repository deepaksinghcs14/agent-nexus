package native

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gatewayservice "github.com/deepaksingh/agent-nexus/services/api/internal/gateway"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WhatsAppToolbox struct {
	pool    *pgxpool.Pool
	cfg     *config.Config
	repo    *repository.GatewayRepository
	service *gatewayservice.Service
	client  *http.Client
}

func NewWhatsAppTools(pool *pgxpool.Pool, cfg *config.Config) []tools.NativeTool {
	tb := &WhatsAppToolbox{
		pool:    pool,
		cfg:     cfg,
		repo:    repository.NewGatewayRepository(pool),
		service: gatewayservice.NewService(pool),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	return []tools.NativeTool{
		&whatsAppSearchContactsTool{tb},
		&whatsAppSendMessageTool{tb},
		&whatsAppCurrentContextTool{tb},
		&whatsAppRecentMessagesTool{tb},
		&whatsAppCreateReminderTool{tb},
		&whatsAppListRemindersTool{tb},
		&whatsAppCompleteReminderTool{tb},
		&whatsAppSummarizeLinkTool{tb},
		&whatsAppOwnerApprovalTool{tb},
		&whatsAppMediaStatusTool{tb},
		&whatsAppScheduleMessageTool{tb},
	}
}

type whatsappContext struct {
	Channel domain.GatewayChannel
	Config  domain.GatewayChannelConfig
	Session domain.ChannelSession
	Contact *domain.GatewayContact
}

func (tb *WhatsAppToolbox) contextFor(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (whatsappContext, error) {
	var out whatsappContext
	channelID := str(input["channel_id"])
	if channelID == "" && execCtx.ChannelSessionID != "" {
		err := tb.pool.QueryRow(ctx, `
			SELECT cs.id::text, cs.workspace_id::text, cs.channel_id::text, cs.account_id, cs.agent_id::text,
			       cs.conversation_id::text, cs.session_key, cs.peer_kind, cs.peer_id, cs.external_sender_id,
			       cs.activation_mode, cs.last_route, cs.last_active_at, cs.created_at
			FROM channel_sessions cs WHERE cs.id=$1::uuid`, execCtx.ChannelSessionID).
			Scan(&out.Session.ID, &out.Session.WorkspaceID, &out.Session.ChannelID, &out.Session.AccountID, &out.Session.AgentID,
				&out.Session.ConversationID, &out.Session.SessionKey, &out.Session.PeerKind, &out.Session.PeerID, &out.Session.ExternalSenderID,
				&out.Session.ActivationMode, &out.Session.LastRoute, &out.Session.LastActiveAt, &out.Session.CreatedAt)
		if err != nil {
			return out, fmt.Errorf("whatsapp context: session not found")
		}
		channelID = out.Session.ChannelID
	}
	if channelID == "" {
		return out, fmt.Errorf("channel_id is required when the run is not from a WhatsApp gateway session")
	}
	c, err := tb.repo.GetChannel(ctx, channelID)
	if err != nil || c.ChannelType != "whatsapp" {
		return out, fmt.Errorf("whatsapp channel not found")
	}
	if execCtx.WorkspaceID != "" && c.WorkspaceID != execCtx.WorkspaceID {
		return out, fmt.Errorf("whatsapp channel not found")
	}
	out.Channel = c
	out.Config = gatewayservice.ParseConfig(c.Config, tb.cfg.WhatsAppAdapterURL)
	if out.Session.ID != "" {
		contact, _ := tb.repo.MatchContact(ctx, c.ID, out.Session.AccountID, out.Session.ExternalSenderID, "")
		out.Contact = contact
	}
	return out, nil
}

func (tb *WhatsAppToolbox) searchContacts(ctx context.Context, wc whatsappContext, query string, limit int) ([]domain.GatewayContact, error) {
	return tb.repo.SearchContacts(ctx, wc.Channel.WorkspaceID, wc.Channel.ID, wc.Config.AccountID, query, limit)
}

func (tb *WhatsAppToolbox) resolveRecipient(ctx context.Context, wc whatsappContext, input map[string]any) (peerID string, contact *domain.GatewayContact, candidates []domain.GatewayContact, err error) {
	if id := str(input["contact_id"]); id != "" {
		c, e := tb.repo.GetContact(ctx, id, wc.Channel.WorkspaceID)
		if e != nil || c.ChannelID != wc.Channel.ID {
			return "", nil, nil, fmt.Errorf("contact not found")
		}
		if c.Role == "blocked" {
			return "", nil, nil, fmt.Errorf("contact is blocked")
		}
		return contactPeer(c), &c, nil, nil
	}
	if jid := str(input["whatsapp_jid"]); jid != "" {
		if blocked, _ := tb.blockedRecipient(ctx, wc, jid, ""); blocked != nil {
			return "", nil, nil, fmt.Errorf("recipient is blocked")
		}
		return jid, nil, nil, nil
	}
	if phone := str(input["phone_number"]); phone != "" {
		phone = gatewayservice.NormalizePhone(phone)
		if blocked, _ := tb.blockedRecipient(ctx, wc, "", phone); blocked != nil {
			return "", nil, nil, fmt.Errorf("recipient is blocked")
		}
		jid := gatewayservice.PhoneToJID(phone)
		if jid == "" {
			return "", nil, nil, fmt.Errorf("valid phone_number is required")
		}
		return jid, nil, nil, nil
	}
	to := str(input["to"])
	if to == "" {
		return "", nil, nil, fmt.Errorf("recipient is required")
	}
	if strings.Contains(to, "@") {
		if blocked, _ := tb.blockedRecipient(ctx, wc, to, ""); blocked != nil {
			return "", nil, nil, fmt.Errorf("recipient is blocked")
		}
		return to, nil, nil, nil
	}
	if looksLikePhone(to) {
		phone := gatewayservice.NormalizePhone(to)
		if blocked, _ := tb.blockedRecipient(ctx, wc, "", phone); blocked != nil {
			return "", nil, nil, fmt.Errorf("recipient is blocked")
		}
		return gatewayservice.PhoneToJID(phone), nil, nil, nil
	}
	found, e := tb.searchContacts(ctx, wc, to, 10)
	if e != nil {
		return "", nil, nil, e
	}
	active := []domain.GatewayContact{}
	for _, c := range found {
		if c.Role == "blocked" {
			continue
		}
		active = append(active, c)
	}
	if len(active) == 0 {
		return "", nil, found, fmt.Errorf("no matching non-blocked contact found")
	}
	if len(active) > 1 {
		return "", nil, active, fmt.Errorf("multiple contacts matched")
	}
	return contactPeer(active[0]), &active[0], nil, nil
}

func (tb *WhatsAppToolbox) blockedRecipient(ctx context.Context, wc whatsappContext, jid, phone string) (*domain.GatewayContact, error) {
	contacts, err := tb.searchContacts(ctx, wc, "", 100)
	if err != nil {
		return nil, err
	}
	phone = gatewayservice.NormalizePhone(phone)
	for _, c := range contacts {
		if c.Role != "blocked" {
			continue
		}
		if jid != "" && c.WhatsAppJID == jid {
			return &c, nil
		}
		if phone != "" && gatewayservice.NormalizePhone(c.PhoneNumber) == phone {
			return &c, nil
		}
	}
	return nil, nil
}

func contactPeer(c domain.GatewayContact) string {
	if c.WhatsAppJID != "" {
		return c.WhatsAppJID
	}
	return gatewayservice.PhoneToJID(c.PhoneNumber)
}

type whatsAppSearchContactsTool struct{ *WhatsAppToolbox }

func (t *whatsAppSearchContactsTool) Definition() domain.Tool {
	return waTool("whatsapp_search_contacts", "Search WhatsApp gateway contacts by name, alias, phone number, or JID.", map[string]any{
		"query":      map[string]any{"type": "string"},
		"channel_id": map[string]any{"type": "string"},
		"limit":      map[string]any{"type": "integer"},
	}, []string{"query"}, "low")
}

func (t *whatsAppSearchContactsTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_search_contacts requires execution context")
}

func (t *whatsAppSearchContactsTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	contacts, err := t.searchContacts(ctx, wc, str(input["query"]), intVal(input["limit"], 10))
	if err != nil {
		return nil, err
	}
	return map[string]any{"contacts": contacts, "count": len(contacts)}, nil
}

type whatsAppSendMessageTool struct{ *WhatsAppToolbox }

func (t *whatsAppSendMessageTool) Definition() domain.Tool {
	return waTool("whatsapp_send_message", "Send a WhatsApp message to a saved contact, name, JID, or raw phone number.", map[string]any{
		"message":      map[string]any{"type": "string"},
		"to":           map[string]any{"type": "string"},
		"contact_id":   map[string]any{"type": "string"},
		"phone_number": map[string]any{"type": "string"},
		"whatsapp_jid": map[string]any{"type": "string"},
		"channel_id":   map[string]any{"type": "string"},
	}, []string{"message"}, "high")
}

func (t *whatsAppSendMessageTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_send_message requires execution context")
}

func (t *whatsAppSendMessageTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	msg := str(input["message"])
	if msg == "" {
		return nil, fmt.Errorf("message is required")
	}
	peerID, contact, candidates, err := t.resolveRecipient(ctx, wc, input)
	if err != nil {
		out := map[string]any{"status": "needs_clarification", "error": err.Error()}
		if len(candidates) > 0 {
			out["candidates"] = candidates
		}
		return out, nil
	}
	out, err := t.service.SendWhatsApp(ctx, gatewayservice.SendRequest{
		Channel: wc.Channel, Config: wc.Config, AccountID: wc.Config.AccountID,
		PeerKind: "direct", PeerID: peerID, Body: msg, SessionID: execCtx.ChannelSessionID, RunID: execCtx.RunID,
	})
	if err != nil {
		return map[string]any{"status": "failed", "error": err.Error(), "outbox_id": out.ID}, nil
	}
	return map[string]any{"status": out.Status, "outbox_id": out.ID, "peer_id": peerID, "contact": contact}, nil
}

type whatsAppCurrentContextTool struct{ *WhatsAppToolbox }

func (t *whatsAppCurrentContextTool) Definition() domain.Tool {
	return waTool("whatsapp_get_current_context", "Get the current WhatsApp gateway channel, session, peer, and contact trust context.", map[string]any{
		"channel_id": map[string]any{"type": "string"},
	}, nil, "low")
}

func (t *whatsAppCurrentContextTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_get_current_context requires execution context")
}

func (t *whatsAppCurrentContextTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": wc.Channel, "session": wc.Session, "contact": wc.Contact}, nil
}

type whatsAppRecentMessagesTool struct{ *WhatsAppToolbox }

func (t *whatsAppRecentMessagesTool) Definition() domain.Tool {
	return waTool("whatsapp_list_recent_messages", "List recent messages in the current WhatsApp conversation.", map[string]any{
		"limit": map[string]any{"type": "integer"},
	}, nil, "low")
}

func (t *whatsAppRecentMessagesTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_list_recent_messages requires execution context")
}

func (t *whatsAppRecentMessagesTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	limit := intVal(input["limit"], 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := t.pool.Query(ctx, `SELECT role, content, created_at FROM messages WHERE conversation_id=$1::uuid ORDER BY created_at DESC LIMIT $2`, execCtx.ConversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var role, content string
		var created time.Time
		if err := rows.Scan(&role, &content, &created); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"role": role, "content": content, "created_at": created})
	}
	return map[string]any{"messages": items}, rows.Err()
}

type whatsAppCreateReminderTool struct{ *WhatsAppToolbox }

func (t *whatsAppCreateReminderTool) Definition() domain.Tool {
	return waTool("whatsapp_create_reminder", "Create a WhatsApp reminder or follow-up task.", map[string]any{
		"title":      map[string]any{"type": "string", "description": "Short label for the reminder (e.g. 'Drink water'). If omitted, derived from message."},
		"message":    map[string]any{"type": "string", "description": "The reminder message text to display when due."},
		"remind_at":  map[string]any{"type": "string", "description": "When to fire the reminder. RFC3339 or natural datetime (e.g. 2026-06-18T15:00:00Z or '2026-06-20 16:47:00 UTC'). Use the current year."},
		"contact_id": map[string]any{"type": "string", "description": "Contact UUID from whatsapp_search_contacts, or the contact's name/alias — the tool will resolve it."},
		"channel_id": map[string]any{"type": "string"},
	}, []string{}, "medium")
}

func (t *whatsAppCreateReminderTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_create_reminder requires execution context")
}

func (t *whatsAppCreateReminderTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	var due *time.Time
	raw := str(input["remind_at"])
	if raw == "" {
		raw = str(input["due_at"]) // legacy alias
	}
	if raw != "" {
		parsed, err := parseFlexibleTime(raw)
		if err != nil {
			return nil, fmt.Errorf("remind_at must be a date/time (e.g. 2026-06-18T15:00:00Z)")
		}
		due = &parsed
	}
	// Resolve contact_id: accept UUID, name/alias, or self-referential terms → use session.
	contactID := str(input["contact_id"])
	if contactID != "" && isSelfReference(contactID) {
		contactID = ""
	}
	if contactID != "" && !isUUID(contactID) {
		contacts, err := t.repo.SearchContacts(ctx, wc.Channel.WorkspaceID, wc.Channel.ID, wc.Config.AccountID, contactID, 1)
		if err != nil || len(contacts) == 0 {
			return nil, fmt.Errorf("contact %q not found — use whatsapp_search_contacts to find the right contact", contactID)
		}
		contactID = contacts[0].ID
	}
	title := str(input["title"])
	if title == "" {
		msg := str(input["message"])
		words := strings.Fields(msg)
		if len(words) > 5 {
			words = words[:5]
		}
		title = strings.Join(words, " ")
		if title == "" {
			title = "Reminder"
		}
	}
	rem := &domain.GatewayReminder{
		WorkspaceID: wc.Channel.WorkspaceID, ChannelID: wc.Channel.ID, SessionID: execCtx.ChannelSessionID,
		ContactID: contactID, AccountID: wc.Config.AccountID, Title: title,
		Message: str(input["message"]), DueAt: due, Status: "pending", Payload: mustJSON(input),
	}
	if err := t.repo.CreateReminder(ctx, rem); err != nil {
		return nil, err
	}
	return rem, nil
}

type whatsAppListRemindersTool struct{ *WhatsAppToolbox }

func (t *whatsAppListRemindersTool) Definition() domain.Tool {
	return waTool("whatsapp_list_reminders", "List WhatsApp reminders and follow-ups.", map[string]any{
		"status":     map[string]any{"type": "string"},
		"channel_id": map[string]any{"type": "string"},
		"limit":      map[string]any{"type": "integer"},
	}, nil, "low")
}

func (t *whatsAppListRemindersTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_list_reminders requires execution context")
}

func (t *whatsAppListRemindersTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	items, err := t.repo.ListReminders(ctx, wc.Channel.WorkspaceID, wc.Channel.ID, str(input["status"]), intVal(input["limit"], 50))
	if err != nil {
		return nil, err
	}
	return map[string]any{"reminders": items}, nil
}

type whatsAppCompleteReminderTool struct{ *WhatsAppToolbox }

func (t *whatsAppCompleteReminderTool) Definition() domain.Tool {
	return waTool("whatsapp_complete_reminder", "Mark a WhatsApp reminder completed or cancelled.", map[string]any{
		"reminder_id": map[string]any{"type": "string"},
		"status":      map[string]any{"type": "string", "enum": []string{"completed", "cancelled"}},
	}, []string{"reminder_id"}, "low")
}

func (t *whatsAppCompleteReminderTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_complete_reminder requires execution context")
}

func (t *whatsAppCompleteReminderTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	status := str(input["status"])
	if status == "" {
		status = "completed"
	}
	rem, err := t.repo.UpdateReminderStatus(ctx, str(input["reminder_id"]), execCtx.WorkspaceID, status)
	if err != nil {
		return nil, err
	}
	return rem, nil
}

type whatsAppSummarizeLinkTool struct{ *WhatsAppToolbox }

func (t *whatsAppSummarizeLinkTool) Definition() domain.Tool {
	return waTool("whatsapp_summarize_link", "Fetch a URL shared in WhatsApp and return a concise text extract for summarization.", map[string]any{
		"url": map[string]any{"type": "string"},
	}, []string{"url"}, "medium")
}

func (t *whatsAppSummarizeLinkTool) Execute(input map[string]any) (any, error) {
	return t.fetch(input)
}

func (t *whatsAppSummarizeLinkTool) fetch(input map[string]any) (any, error) {
	u := str(input["url"])
	if u == "" {
		return nil, fmt.Errorf("url is required")
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 20000))
	return map[string]any{"status": res.StatusCode, "url": u, "text": string(body)}, nil
}

type whatsAppOwnerApprovalTool struct{ *WhatsAppToolbox }

func (t *whatsAppOwnerApprovalTool) Definition() domain.Tool {
	return waTool("whatsapp_request_owner_approval", "Create an owner escalation for a risky or ambiguous WhatsApp action.", map[string]any{
		"action_type": map[string]any{"type": "string"},
		"recipient":   map[string]any{"type": "string"},
		"message":     map[string]any{"type": "string"},
		"reason":      map[string]any{"type": "string"},
		"channel_id":  map[string]any{"type": "string"},
	}, []string{"action_type", "reason"}, "medium")
}

func (t *whatsAppOwnerApprovalTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_request_owner_approval requires execution context")
}

func (t *whatsAppOwnerApprovalTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	e := &domain.GatewayEscalation{
		WorkspaceID: wc.Channel.WorkspaceID, ChannelID: wc.Channel.ID, SessionID: execCtx.ChannelSessionID, RunID: execCtx.RunID,
		AccountID: wc.Config.AccountID, ActionType: str(input["action_type"]), Recipient: str(input["recipient"]),
		Message: str(input["message"]), Reason: str(input["reason"]), Status: "pending", ApprovalCode: approvalCode(), Payload: mustJSON(input),
	}
	if err := t.repo.CreateEscalation(ctx, e); err != nil {
		return nil, err
	}
	notified := 0
	owners, _ := t.repo.ListOwnerContacts(ctx, wc.Channel.ID, wc.Config.AccountID)
	for _, owner := range owners {
		peer := contactPeer(owner)
		if peer == "" {
			continue
		}
		body := ownerApprovalMessage(e)
		if _, err := t.service.SendWhatsApp(ctx, gatewayservice.SendRequest{
			Channel: wc.Channel, Config: wc.Config, AccountID: wc.Config.AccountID,
			PeerKind: "direct", PeerID: peer, Body: body, SessionID: execCtx.ChannelSessionID, RunID: execCtx.RunID,
		}); err == nil {
			notified++
		}
	}
	return map[string]any{"escalation": e, "approval_code": e.ApprovalCode, "notified_owners": notified}, nil
}

type whatsAppMediaStatusTool struct{ *WhatsAppToolbox }

func (t *whatsAppMediaStatusTool) Definition() domain.Tool {
	return waTool("whatsapp_send_media_status", "Return the current WhatsApp media support status.", map[string]any{
		"media_type": map[string]any{"type": "string"},
	}, nil, "low")
}

func (t *whatsAppMediaStatusTool) Execute(input map[string]any) (any, error) {
	return map[string]any{
		"supported": false,
		"message":   "Media extraction is not available yet. Ask the user to provide text or a caption.",
	}, nil
}

func approvalCode() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%02X%02X%02X", b[0], b[1], b[2])
}

func ownerApprovalMessage(e *domain.GatewayEscalation) string {
	var b strings.Builder
	b.WriteString("Approval needed: " + waDefault(e.ActionType, "whatsapp_action") + "\n")
	b.WriteString("Code: " + e.ApprovalCode + "\n")
	if e.Recipient != "" {
		b.WriteString("Recipient: " + e.Recipient + "\n")
	}
	if e.Reason != "" {
		b.WriteString("Reason: " + e.Reason + "\n")
	}
	if e.Message != "" {
		b.WriteString("Message: " + waTruncate(e.Message, 500) + "\n")
	}
	b.WriteString("\nReply approve " + e.ApprovalCode + " or reject " + e.ApprovalCode + ".\n")
	b.WriteString("Reply disable approvals to turn off chat approvals.")
	return b.String()
}

func waDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func waTruncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func waTool(name, description string, props map[string]any, required []string, risk string) domain.Tool {
	schema, _ := json.Marshal(map[string]any{"type": "object", "properties": props, "required": required})
	return domain.Tool{
		Name: name, Description: description, Type: "native", InputSchema: schema,
		RiskLevel: risk, RequiresApproval: false, TimeoutMs: 30000, Enabled: true,
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func intVal(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return fallback
	}
}

func looksLikePhone(s string) bool {
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 7
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

type whatsAppScheduleMessageTool struct{ *WhatsAppToolbox }

func (t *whatsAppScheduleMessageTool) Definition() domain.Tool {
	return waTool("whatsapp_schedule_message", "Schedule a WhatsApp message to be sent to a contact at a specific time. Supports one-time and recurring sends.", map[string]any{
		"message":                  map[string]any{"type": "string", "description": "Text to send."},
		"to":                       map[string]any{"type": "string", "description": "Contact name, alias, phone, or JID."},
		"contact_id":               map[string]any{"type": "string", "description": "Contact UUID from whatsapp_search_contacts."},
		"send_at":                  map[string]any{"type": "string", "description": "RFC3339 timestamp when to send (use the current year)."},
		"recurrence_frequency":     map[string]any{"type": "string", "enum": []string{"daily", "weekly", "monthly", "weekdays"}, "description": "Omit for one-time."},
		"recurrence_interval":      map[string]any{"type": "integer", "description": "Repeat every N periods, default 1."},
		"recurrence_end_at":        map[string]any{"type": "string", "description": "RFC3339 date after which to stop."},
		"recurrence_max_occurrences": map[string]any{"type": "integer", "description": "Max number of times to send."},
		"channel_id":               map[string]any{"type": "string"},
	}, []string{"message", "send_at"}, "medium")
}

func (t *whatsAppScheduleMessageTool) Execute(input map[string]any) (any, error) {
	return nil, fmt.Errorf("whatsapp_schedule_message requires execution context")
}

func (t *whatsAppScheduleMessageTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	wc, err := t.contextFor(ctx, execCtx, input)
	if err != nil {
		return nil, err
	}
	msg := str(input["message"])
	if msg == "" {
		return nil, fmt.Errorf("message is required")
	}
	rawSendAt := str(input["send_at"])
	if rawSendAt == "" {
		return nil, fmt.Errorf("send_at is required")
	}
	sendAt, err := parseFlexibleTime(rawSendAt)
	if err != nil {
		return nil, fmt.Errorf("send_at must be a date/time (e.g. 2026-06-18T15:00:00Z)")
	}
	peerID, contact, candidates, err := t.resolveRecipient(ctx, wc, input)
	if err != nil {
		out := map[string]any{"status": "needs_clarification", "error": err.Error()}
		if len(candidates) > 0 {
			out["candidates"] = candidates
		}
		return out, nil
	}
	contactID := ""
	if contact != nil {
		contactID = contact.ID
	}

	// Build recurrence rule if frequency is provided.
	var recurrenceRule json.RawMessage
	if freq := str(input["recurrence_frequency"]); freq != "" {
		rule := map[string]any{"frequency": freq}
		if iv := intVal(input["recurrence_interval"], 0); iv > 0 {
			rule["interval"] = iv
		}
		if endAt := str(input["recurrence_end_at"]); endAt != "" {
			rule["end_at"] = endAt
		}
		if maxOcc := intVal(input["recurrence_max_occurrences"], 0); maxOcc > 0 {
			rule["max_occurrences"] = maxOcc
		}
		recurrenceRule, _ = json.Marshal(rule)
	}

	m := &domain.ScheduledMessage{
		WorkspaceID:    wc.Channel.WorkspaceID,
		ChannelID:      wc.Channel.ID,
		ContactID:      contactID,
		AccountID:      wc.Config.AccountID,
		PeerKind:       "direct",
		PeerID:         peerID,
		Message:        msg,
		SendAt:         sendAt,
		Status:         "pending",
		RecurrenceRule: recurrenceRule,
		CreatedBy:      execCtx.RunID,
	}
	if err := t.repo.CreateScheduledMessage(ctx, m); err != nil {
		return nil, err
	}
	return map[string]any{"scheduled_message": m, "status": "scheduled", "send_at": sendAt}, nil
}

// parseFlexibleTime tries RFC3339, then bare datetime without timezone (treated as UTC).
func parseFlexibleTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05 UTC",
		"2006-01-02 15:04 UTC",
		"2006-01-02T15:04:05 UTC",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	// Strip a trailing " UTC" suffix and retry as bare datetime.
	if after, ok := strings.CutSuffix(strings.TrimSpace(s), " UTC"); ok {
		return parseFlexibleTime(after)
	}
	return time.Time{}, fmt.Errorf("unrecognised time format: %q", s)
}

// isSelfReference returns true when the LLM passes "me", "myself", etc. as a contact_id.
func isSelfReference(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "me", "myself", "i", "self", "current", "current user", "current contact":
		return true
	}
	return false
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
