package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GatewayRepository struct{ pool *pgxpool.Pool }

func NewGatewayRepository(pool *pgxpool.Pool) *GatewayRepository {
	return &GatewayRepository{pool: pool}
}

const gatewayContactSelect = `
SELECT id::text, workspace_id::text, channel_id::text, account_id, display_name, alias,
       phone_number, whatsapp_jid, role, COALESCE(agent_id::text,''), auto_reply_enabled,
       last_matched_at, created_at, updated_at
FROM gateway_contacts`

func scanGatewayContact(row interface{ Scan(...any) error }) (domain.GatewayContact, error) {
	var c domain.GatewayContact
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.ChannelID, &c.AccountID, &c.DisplayName, &c.Alias,
		&c.PhoneNumber, &c.WhatsAppJID, &c.Role, &c.AgentID, &c.AutoReplyEnabled,
		&c.LastMatchedAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const gatewayChannelSelect = `
SELECT gc.id::text, gc.workspace_id::text, gc.agent_id::text, COALESCE(a.name,''), gc.name, gc.description,
       gc.channel_type, gc.config, gc.is_active, gc.created_by::text, gc.created_at, gc.updated_at
FROM gateway_channels gc
LEFT JOIN agents a ON a.id=gc.agent_id`

func scanGatewayChannel(row interface{ Scan(...any) error }) (domain.GatewayChannel, error) {
	var c domain.GatewayChannel
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.AgentID, &c.AgentName, &c.Name, &c.Description, &c.ChannelType, &c.Config, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *GatewayRepository) ListAllActiveWhatsAppChannels(ctx context.Context) ([]domain.GatewayChannel, error) {
	rows, err := r.pool.Query(ctx, gatewayChannelSelect+` WHERE gc.channel_type='whatsapp' AND gc.is_active=true ORDER BY gc.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayChannel{}
	for rows.Next() {
		c, err := scanGatewayChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) ListChannels(ctx context.Context, workspaceID string) ([]domain.GatewayChannel, error) {
	rows, err := r.pool.Query(ctx, gatewayChannelSelect+` WHERE gc.workspace_id=$1::uuid ORDER BY gc.created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayChannel{}
	for rows.Next() {
		c, err := scanGatewayChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) GetChannel(ctx context.Context, id string) (domain.GatewayChannel, error) {
	return scanGatewayChannel(r.pool.QueryRow(ctx, gatewayChannelSelect+` WHERE gc.id=$1::uuid`, id))
}

func (r *GatewayRepository) GetChannelInWorkspace(ctx context.Context, id, workspaceID string) (domain.GatewayChannel, error) {
	return scanGatewayChannel(r.pool.QueryRow(ctx, gatewayChannelSelect+` WHERE gc.id=$1::uuid AND gc.workspace_id=$2::uuid`, id, workspaceID))
}

func (r *GatewayRepository) CreateChannel(ctx context.Context, c *domain.GatewayChannel) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO gateway_channels(id,workspace_id,agent_id,name,description,channel_type,config,is_active,created_by)
		 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9::uuid)
		 RETURNING created_at, updated_at`,
		c.ID, c.WorkspaceID, c.AgentID, c.Name, c.Description, c.ChannelType, c.Config, c.IsActive, c.CreatedBy,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

func (r *GatewayRepository) UpdateChannel(ctx context.Context, c *domain.GatewayChannel) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE gateway_channels SET agent_id=$1::uuid, name=$2, description=$3, config=$4, is_active=$5, updated_at=NOW()
		 WHERE id=$6::uuid AND workspace_id=$7::uuid`,
		c.AgentID, c.Name, c.Description, c.Config, c.IsActive, c.ID, c.WorkspaceID)
	return err
}

func (r *GatewayRepository) DeleteChannel(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM gateway_channels WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return err
}

func (r *GatewayRepository) UpsertAccount(ctx context.Context, a *domain.GatewayChannelAccount) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_channel_accounts(workspace_id,channel_id,account_id,status,self_id,last_error,last_seen_at)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7)
		ON CONFLICT(channel_id, account_id) DO UPDATE SET
			status=EXCLUDED.status, self_id=EXCLUDED.self_id, last_error=EXCLUDED.last_error,
			last_seen_at=EXCLUDED.last_seen_at, updated_at=NOW()
		RETURNING id::text, created_at, updated_at`,
		a.WorkspaceID, a.ChannelID, a.AccountID, a.Status, a.SelfID, a.LastError, a.LastSeenAt,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *GatewayRepository) ListAccounts(ctx context.Context, workspaceID, channelID string) ([]domain.GatewayChannelAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, account_id, status, self_id, last_error, last_seen_at, created_at, updated_at
		FROM gateway_channel_accounts
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		ORDER BY updated_at DESC`, workspaceID, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayChannelAccount{}
	for rows.Next() {
		var a domain.GatewayChannelAccount
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.ChannelID, &a.AccountID, &a.Status, &a.SelfID, &a.LastError, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) UpsertSession(ctx context.Context, s *domain.ChannelSession) (domain.ChannelSession, bool, error) {
	var out domain.ChannelSession
	var inserted bool
	err := r.pool.QueryRow(ctx, `
		INSERT INTO channel_sessions(workspace_id,channel_id,account_id,agent_id,conversation_id,session_key,peer_kind,peer_id,external_sender_id,activation_mode,last_route)
		VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11)
		ON CONFLICT(channel_id, account_id, session_key) DO UPDATE SET
			last_active_at=NOW(), last_route=EXCLUDED.last_route, external_sender_id=EXCLUDED.external_sender_id
		RETURNING id::text, workspace_id::text, channel_id::text, account_id, agent_id::text, conversation_id::text,
		          session_key, peer_kind, peer_id, external_sender_id, activation_mode, last_route, last_active_at, created_at,
		          (xmax = 0) AS inserted`,
		s.WorkspaceID, s.ChannelID, s.AccountID, s.AgentID, s.ConversationID, s.SessionKey, s.PeerKind, s.PeerID, s.ExternalSenderID, s.ActivationMode, s.LastRoute,
	).Scan(&out.ID, &out.WorkspaceID, &out.ChannelID, &out.AccountID, &out.AgentID, &out.ConversationID, &out.SessionKey, &out.PeerKind, &out.PeerID, &out.ExternalSenderID, &out.ActivationMode, &out.LastRoute, &out.LastActiveAt, &out.CreatedAt, &inserted)
	return out, inserted, err
}

func (r *GatewayRepository) ListSessions(ctx context.Context, workspaceID, channelID string, limit int) ([]domain.ChannelSession, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, account_id, agent_id::text, conversation_id::text,
		       session_key, peer_kind, peer_id, external_sender_id, activation_mode, last_route, last_active_at, created_at
		FROM channel_sessions
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		ORDER BY last_active_at DESC LIMIT $3`, workspaceID, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ChannelSession{}
	for rows.Next() {
		var s domain.ChannelSession
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.ChannelID, &s.AccountID, &s.AgentID, &s.ConversationID, &s.SessionKey, &s.PeerKind, &s.PeerID, &s.ExternalSenderID, &s.ActivationMode, &s.LastRoute, &s.LastActiveAt, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) DeleteSession(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM channel_sessions WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return err
}

func (r *GatewayRepository) CreateEvent(ctx context.Context, e *domain.GatewayEvent) error {
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_events(workspace_id,channel_id,session_id,run_id,event_type,provider_message_id,payload)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7)
		ON CONFLICT(channel_id, provider_message_id) WHERE provider_message_id IS NOT NULL DO NOTHING
		RETURNING id::text, created_at`,
		e.WorkspaceID, e.ChannelID, nullableString(e.SessionID), nullableString(e.RunID), e.EventType, nullableString(e.ProviderMessageID), payload,
	).Scan(&e.ID, &e.CreatedAt)
}

func (r *GatewayRepository) HasEventForProviderMessage(ctx context.Context, channelID, providerMessageID string) (bool, error) {
	if providerMessageID == "" {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM gateway_events WHERE channel_id=$1::uuid AND provider_message_id=$2)`, channelID, providerMessageID).Scan(&exists)
	return exists, err
}

func (r *GatewayRepository) ListEvents(ctx context.Context, workspaceID, channelID string, limit int) ([]domain.GatewayEvent, error) {
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, COALESCE(session_id::text,''), COALESCE(run_id::text,''),
		       event_type, COALESCE(provider_message_id,''), payload, created_at
		FROM gateway_events
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		ORDER BY created_at DESC LIMIT $3`, workspaceID, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayEvent{}
	for rows.Next() {
		var e domain.GatewayEvent
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.ChannelID, &e.SessionID, &e.RunID, &e.EventType, &e.ProviderMessageID, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) CreatePairing(ctx context.Context, p *domain.GatewayPairingRequest) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_pairing_requests(workspace_id,channel_id,account_id,peer_kind,peer_id,sender_id,code,status,expires_at)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,'pending',$8)
		ON CONFLICT(channel_id, account_id, sender_id, status) DO UPDATE SET code=EXCLUDED.code, expires_at=EXCLUDED.expires_at, created_at=NOW()
		RETURNING id::text, created_at`,
		p.WorkspaceID, p.ChannelID, p.AccountID, p.PeerKind, p.PeerID, p.SenderID, p.Code, p.ExpiresAt,
	).Scan(&p.ID, &p.CreatedAt)
}

func (r *GatewayRepository) ListPairings(ctx context.Context, workspaceID, channelID, status string) ([]domain.GatewayPairingRequest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, account_id, peer_kind, peer_id, sender_id, code, status, expires_at, created_at
		FROM gateway_pairing_requests
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid) AND ($3='' OR status=$3)
		ORDER BY created_at DESC`, workspaceID, channelID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayPairingRequest{}
	for rows.Next() {
		var p domain.GatewayPairingRequest
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.ChannelID, &p.AccountID, &p.PeerKind, &p.PeerID, &p.SenderID, &p.Code, &p.Status, &p.ExpiresAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) UpdatePairingStatus(ctx context.Context, id, workspaceID, status string) (domain.GatewayPairingRequest, error) {
	var p domain.GatewayPairingRequest
	err := r.pool.QueryRow(ctx, `
		UPDATE gateway_pairing_requests SET status=$1
		WHERE id=$2::uuid AND workspace_id=$3::uuid
		RETURNING id::text, workspace_id::text, channel_id::text, account_id, peer_kind, peer_id, sender_id, code, status, expires_at, created_at`,
		status, id, workspaceID,
	).Scan(&p.ID, &p.WorkspaceID, &p.ChannelID, &p.AccountID, &p.PeerKind, &p.PeerID, &p.SenderID, &p.Code, &p.Status, &p.ExpiresAt, &p.CreatedAt)
	return p, err
}

func (r *GatewayRepository) CreateOutbound(ctx context.Context, m *domain.GatewayOutboundMessage) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_outbound_messages(workspace_id,channel_id,session_id,run_id,account_id,peer_kind,peer_id,body,status)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9)
		RETURNING id::text, created_at`,
		m.WorkspaceID, m.ChannelID, nullableString(m.SessionID), nullableString(m.RunID), m.AccountID, m.PeerKind, m.PeerID, m.Body, m.Status,
	).Scan(&m.ID, &m.CreatedAt)
}

func (r *GatewayRepository) ListOutbox(ctx context.Context, workspaceID, channelID string, limit int) ([]domain.GatewayOutboundMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, COALESCE(session_id::text,''), COALESCE(run_id::text,''),
		       account_id, peer_kind, peer_id, body, status, attempts, last_error, created_at, sent_at
		FROM gateway_outbound_messages
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		ORDER BY created_at DESC LIMIT $3`, workspaceID, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayOutboundMessage{}
	for rows.Next() {
		var m domain.GatewayOutboundMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.SessionID, &m.RunID, &m.AccountID, &m.PeerKind, &m.PeerID, &m.Body, &m.Status, &m.Attempts, &m.LastError, &m.CreatedAt, &m.SentAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) MarkOutbound(ctx context.Context, id, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE gateway_outbound_messages
		SET status=$2, attempts=attempts+1, last_error=$3, sent_at=CASE WHEN $2='sent' THEN NOW() ELSE sent_at END
		WHERE id=$1::uuid`, id, status, errMsg)
	return err
}

func (r *GatewayRepository) ListContacts(ctx context.Context, workspaceID, channelID string) ([]domain.GatewayContact, error) {
	rows, err := r.pool.Query(ctx, gatewayContactSelect+`
		WHERE workspace_id=$1::uuid AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		ORDER BY role ASC, display_name ASC`, workspaceID, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayContact{}
	for rows.Next() {
		c, err := scanGatewayContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) GetContact(ctx context.Context, id, workspaceID string) (domain.GatewayContact, error) {
	return scanGatewayContact(r.pool.QueryRow(ctx, gatewayContactSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID))
}

func (r *GatewayRepository) CreateContact(ctx context.Context, c *domain.GatewayContact) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_contacts(workspace_id,channel_id,account_id,display_name,alias,phone_number,whatsapp_jid,role,agent_id,auto_reply_enabled)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::uuid,$10)
		RETURNING id::text, created_at, updated_at`,
		c.WorkspaceID, c.ChannelID, c.AccountID, c.DisplayName, c.Alias, normalizePhone(c.PhoneNumber),
		c.WhatsAppJID, c.Role, nullableString(c.AgentID), c.AutoReplyEnabled,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *GatewayRepository) UpdateContact(ctx context.Context, c *domain.GatewayContact) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE gateway_contacts
		SET display_name=$1, alias=$2, phone_number=$3, whatsapp_jid=$4, role=$5,
		    agent_id=$6::uuid, auto_reply_enabled=$7, updated_at=NOW()
		WHERE id=$8::uuid AND workspace_id=$9::uuid`,
		c.DisplayName, c.Alias, normalizePhone(c.PhoneNumber), c.WhatsAppJID, c.Role,
		nullableString(c.AgentID), c.AutoReplyEnabled, c.ID, c.WorkspaceID)
	return err
}

func (r *GatewayRepository) DeleteContact(ctx context.Context, id, workspaceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM gateway_contacts WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, workspaceID)
	return err
}

func (r *GatewayRepository) MatchContact(ctx context.Context, channelID, accountID, senderJID, senderPhone string) (*domain.GatewayContact, error) {
	senderPhone = normalizePhone(senderPhone)
	row := r.pool.QueryRow(ctx, gatewayContactSelect+`
		WHERE channel_id=$1::uuid
		  AND account_id=$2
		  AND (
		    (whatsapp_jid <> '' AND whatsapp_jid=$3)
		    OR (phone_number <> '' AND phone_number=$4)
		  )
		ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'trusted' THEN 1 ELSE 2 END
		LIMIT 1`, channelID, accountID, senderJID, senderPhone)
	c, err := scanGatewayContact(row)
	if err != nil {
		return nil, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE gateway_contacts SET last_matched_at=NOW() WHERE id=$1::uuid`, c.ID)
	return &c, nil
}

func (r *GatewayRepository) SearchContacts(ctx context.Context, workspaceID, channelID, accountID, query string, limit int) ([]domain.GatewayContact, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	q := strings.ToLower(strings.TrimSpace(query))
	rows, err := r.pool.Query(ctx, gatewayContactSelect+`
		WHERE workspace_id=$1::uuid
		  AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		  AND (NULLIF($3,'') IS NULL OR account_id=$3)
		  AND (
		    $4=''
		    OR lower(display_name) LIKE '%' || $4 || '%'
		    OR lower(alias) LIKE '%' || $4 || '%'
		    OR phone_number LIKE '%' || $4 || '%'
		    OR lower(whatsapp_jid) LIKE '%' || $4 || '%'
		  )
		ORDER BY
		  CASE WHEN lower(alias)=$4 THEN 0 WHEN lower(display_name)=$4 THEN 1 ELSE 2 END,
		  role ASC, display_name ASC
		LIMIT $5`, workspaceID, channelID, accountID, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayContact{}
	for rows.Next() {
		c, err := scanGatewayContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) ListContactPhones(ctx context.Context, channelID, accountID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT phone_number FROM gateway_contacts WHERE channel_id=$1::uuid AND account_id=$2 AND phone_number<>'' AND role<>'owner'`,
		channelID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var phones []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if p != "" {
			phones = append(phones, p)
		}
	}
	return phones, rows.Err()
}

func (r *GatewayRepository) CreateReminder(ctx context.Context, m *domain.GatewayReminder) error {
	payload := m.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_reminders(workspace_id,channel_id,session_id,contact_id,account_id,title,message,due_at,status,payload)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10)
		RETURNING id::text, created_at, updated_at`,
		m.WorkspaceID, nullableString(m.ChannelID), nullableString(m.SessionID), nullableString(m.ContactID),
		m.AccountID, m.Title, m.Message, m.DueAt, defaultStatus(m.Status, "pending"), payload,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (r *GatewayRepository) ListReminders(ctx context.Context, workspaceID, channelID, status string, limit int) ([]domain.GatewayReminder, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, COALESCE(channel_id::text,''), COALESCE(session_id::text,''),
		       COALESCE(contact_id::text,''), account_id, title, message, due_at, status, payload, created_at, updated_at
		FROM gateway_reminders
		WHERE workspace_id=$1::uuid
		  AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		  AND ($3='' OR status=$3)
		ORDER BY COALESCE(due_at, created_at) ASC
		LIMIT $4`, workspaceID, channelID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayReminder{}
	for rows.Next() {
		var m domain.GatewayReminder
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.SessionID, &m.ContactID, &m.AccountID, &m.Title, &m.Message, &m.DueAt, &m.Status, &m.Payload, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type DueReminder struct {
	domain.GatewayReminder
	PeerKind string
	PeerID   string
}

func (r *GatewayRepository) FetchDueReminders(ctx context.Context) ([]DueReminder, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
		  r.id::text, r.workspace_id::text, COALESCE(r.channel_id::text,''),
		  COALESCE(r.session_id::text,''), COALESCE(r.contact_id::text,''),
		  r.account_id, r.title, r.message, r.due_at, r.status, r.payload, r.created_at, r.updated_at,
		  COALESCE(s.peer_kind,'direct'),
		  COALESCE(s.peer_id, cont.whatsapp_jid, '')
		FROM gateway_reminders r
		JOIN gateway_channels gc ON gc.id = r.channel_id AND gc.is_active = true
		LEFT JOIN channel_sessions s ON s.id = r.session_id
		LEFT JOIN gateway_contacts cont ON cont.id = r.contact_id
		WHERE r.status = 'pending'
		  AND r.due_at IS NOT NULL
		  AND r.due_at <= NOW()
		  AND r.channel_id IS NOT NULL
		ORDER BY r.due_at ASC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueReminder
	for rows.Next() {
		var d DueReminder
		if err := rows.Scan(
			&d.ID, &d.WorkspaceID, &d.ChannelID, &d.SessionID, &d.ContactID,
			&d.AccountID, &d.Title, &d.Message, &d.DueAt, &d.Status, &d.Payload, &d.CreatedAt, &d.UpdatedAt,
			&d.PeerKind, &d.PeerID,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) UpdateReminderStatus(ctx context.Context, id, workspaceID, status string) (domain.GatewayReminder, error) {
	var m domain.GatewayReminder
	err := r.pool.QueryRow(ctx, `
		UPDATE gateway_reminders SET status=$1, updated_at=NOW()
		WHERE id=$2::uuid AND workspace_id=$3::uuid
		RETURNING id::text, workspace_id::text, COALESCE(channel_id::text,''), COALESCE(session_id::text,''),
		          COALESCE(contact_id::text,''), account_id, title, message, due_at, status, payload, created_at, updated_at`,
		status, id, workspaceID,
	).Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.SessionID, &m.ContactID, &m.AccountID, &m.Title, &m.Message, &m.DueAt, &m.Status, &m.Payload, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (r *GatewayRepository) CreateEscalation(ctx context.Context, e *domain.GatewayEscalation) error {
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_escalations(workspace_id,channel_id,session_id,run_id,account_id,action_type,recipient,message,reason,status,approval_code,payload)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text, created_at, updated_at`,
		e.WorkspaceID, nullableString(e.ChannelID), nullableString(e.SessionID), nullableString(e.RunID),
		e.AccountID, e.ActionType, e.Recipient, e.Message, e.Reason, defaultStatus(e.Status, "pending"), e.ApprovalCode, payload,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

func (r *GatewayRepository) ListEscalations(ctx context.Context, workspaceID, channelID, status string, limit int) ([]domain.GatewayEscalation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, COALESCE(channel_id::text,''), COALESCE(session_id::text,''),
		       COALESCE(run_id::text,''), account_id, action_type, recipient, message, reason, status,
		       approval_code, resolved_by_sender_id, resolved_at, payload, created_at, updated_at
		FROM gateway_escalations
		WHERE workspace_id=$1::uuid
		  AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		  AND ($3='' OR status=$3)
		ORDER BY created_at DESC
		LIMIT $4`, workspaceID, channelID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayEscalation{}
	for rows.Next() {
		var e domain.GatewayEscalation
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.ChannelID, &e.SessionID, &e.RunID, &e.AccountID, &e.ActionType, &e.Recipient, &e.Message, &e.Reason, &e.Status, &e.ApprovalCode, &e.ResolvedBySenderID, &e.ResolvedAt, &e.Payload, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) ResolveEscalationByID(ctx context.Context, id, workspaceID, status, senderID string) (domain.GatewayEscalation, error) {
	var e domain.GatewayEscalation
	err := r.pool.QueryRow(ctx, `
		UPDATE gateway_escalations
		SET status=$1, resolved_by_sender_id=$2, resolved_at=NOW(), updated_at=NOW()
		WHERE id=$3::uuid AND workspace_id=$4::uuid AND status='pending'
		RETURNING id::text, workspace_id::text, COALESCE(channel_id::text,''), COALESCE(session_id::text,''),
		          COALESCE(run_id::text,''), account_id, action_type, recipient, message, reason, status,
		          approval_code, resolved_by_sender_id, resolved_at, payload, created_at, updated_at`,
		status, senderID, id, workspaceID,
	).Scan(&e.ID, &e.WorkspaceID, &e.ChannelID, &e.SessionID, &e.RunID, &e.AccountID, &e.ActionType, &e.Recipient, &e.Message, &e.Reason, &e.Status, &e.ApprovalCode, &e.ResolvedBySenderID, &e.ResolvedAt, &e.Payload, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (r *GatewayRepository) ResolveEscalationByCode(ctx context.Context, channelID, code, status, senderID string) (domain.GatewayEscalation, error) {
	var e domain.GatewayEscalation
	err := r.pool.QueryRow(ctx, `
		UPDATE gateway_escalations
		SET status=$1, resolved_by_sender_id=$2, resolved_at=NOW(), updated_at=NOW()
		WHERE channel_id=$3::uuid AND upper(approval_code)=upper($4) AND status='pending'
		RETURNING id::text, workspace_id::text, COALESCE(channel_id::text,''), COALESCE(session_id::text,''),
		          COALESCE(run_id::text,''), account_id, action_type, recipient, message, reason, status,
		          approval_code, resolved_by_sender_id, resolved_at, payload, created_at, updated_at`,
		status, senderID, channelID, code,
	).Scan(&e.ID, &e.WorkspaceID, &e.ChannelID, &e.SessionID, &e.RunID, &e.AccountID, &e.ActionType, &e.Recipient, &e.Message, &e.Reason, &e.Status, &e.ApprovalCode, &e.ResolvedBySenderID, &e.ResolvedAt, &e.Payload, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (r *GatewayRepository) ListOwnerContacts(ctx context.Context, channelID, accountID string) ([]domain.GatewayContact, error) {
	rows, err := r.pool.Query(ctx, gatewayContactSelect+`
		WHERE channel_id=$1::uuid AND account_id=$2 AND role='owner' AND auto_reply_enabled=true
		ORDER BY display_name`, channelID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GatewayContact{}
	for rows.Next() {
		c, err := scanGatewayContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func normalizePhone(s string) string {
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

func defaultStatus(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (r *GatewayRepository) Tx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// ─── Scheduled Messages ────────────────────────────────────────────────────

func (r *GatewayRepository) CreateScheduledMessage(ctx context.Context, m *domain.ScheduledMessage) error {
	rule := m.RecurrenceRule
	if len(rule) == 0 {
		rule = nil
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO gateway_scheduled_messages
		  (workspace_id,channel_id,contact_id,account_id,peer_kind,peer_id,message,send_at,status,recurrence_rule,occurrence_count,created_by)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text, created_at, updated_at`,
		m.WorkspaceID, m.ChannelID, nullableString(m.ContactID),
		m.AccountID, m.PeerKind, m.PeerID, m.Message, m.SendAt,
		defaultStatus(m.Status, "pending"), rule, m.OccurrenceCount, m.CreatedBy,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (r *GatewayRepository) ListScheduledMessages(ctx context.Context, workspaceID, channelID, status string, limit int) ([]domain.ScheduledMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, COALESCE(contact_id::text,''),
		       account_id, peer_kind, peer_id, message, send_at, status,
		       recurrence_rule, occurrence_count, last_error, created_by, created_at, updated_at
		FROM gateway_scheduled_messages
		WHERE workspace_id=$1::uuid
		  AND (NULLIF($2,'') IS NULL OR channel_id=NULLIF($2,'')::uuid)
		  AND ($3='' OR status=$3)
		ORDER BY send_at ASC
		LIMIT $4`, workspaceID, channelID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ScheduledMessage
	for rows.Next() {
		var m domain.ScheduledMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.ContactID,
			&m.AccountID, &m.PeerKind, &m.PeerID, &m.Message, &m.SendAt, &m.Status,
			&m.RecurrenceRule, &m.OccurrenceCount, &m.LastError, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *GatewayRepository) GetScheduledMessage(ctx context.Context, id, workspaceID string) (domain.ScheduledMessage, error) {
	var m domain.ScheduledMessage
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, workspace_id::text, channel_id::text, COALESCE(contact_id::text,''),
		       account_id, peer_kind, peer_id, message, send_at, status,
		       recurrence_rule, occurrence_count, last_error, created_by, created_at, updated_at
		FROM gateway_scheduled_messages
		WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		id, workspaceID,
	).Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.ContactID,
		&m.AccountID, &m.PeerKind, &m.PeerID, &m.Message, &m.SendAt, &m.Status,
		&m.RecurrenceRule, &m.OccurrenceCount, &m.LastError, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (r *GatewayRepository) UpdateScheduledMessageStatus(ctx context.Context, id, workspaceID, status, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE gateway_scheduled_messages SET status=$1, last_error=$2, updated_at=NOW()
		WHERE id=$3::uuid AND workspace_id=$4::uuid`,
		status, lastError, id, workspaceID)
	return err
}

func (r *GatewayRepository) RescheduleMessage(ctx context.Context, id, workspaceID string, nextSendAt time.Time, occurrenceCount int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE gateway_scheduled_messages SET send_at=$1, occurrence_count=$2, updated_at=NOW()
		WHERE id=$3::uuid AND workspace_id=$4::uuid`,
		nextSendAt, occurrenceCount, id, workspaceID)
	return err
}

func (r *GatewayRepository) FetchDueScheduledMessages(ctx context.Context) ([]domain.ScheduledMessage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sm.id::text, sm.workspace_id::text, sm.channel_id::text, COALESCE(sm.contact_id::text,''),
		       sm.account_id, sm.peer_kind, sm.peer_id, sm.message, sm.send_at, sm.status,
		       sm.recurrence_rule, sm.occurrence_count, sm.last_error, sm.created_by, sm.created_at, sm.updated_at
		FROM gateway_scheduled_messages sm
		JOIN gateway_channels gc ON gc.id = sm.channel_id AND gc.is_active = true
		WHERE sm.status = 'pending'
		  AND sm.send_at <= NOW()
		ORDER BY sm.send_at ASC
		LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ScheduledMessage
	for rows.Next() {
		var m domain.ScheduledMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &m.ContactID,
			&m.AccountID, &m.PeerKind, &m.PeerID, &m.Message, &m.SendAt, &m.Status,
			&m.RecurrenceRule, &m.OccurrenceCount, &m.LastError, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
