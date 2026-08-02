package handler

// "pairing" is the default DM policy for every new WhatsApp channel
// (normalizeGatewayConfig), and it used to mean "silently drop every
// stranger forever" — senderAllowed treated it identically to "allowlist",
// and CreatePairing had zero callers anywhere in the codebase, so the
// Pairing tab was permanently empty. DB-backed because the thing that broke
// is a repository write, and handleInbound is exercised end to end: an
// unknown sender is rejected at the policy gate and never reaches
// dispatchGatewayRun, so no LLM or runner involvement is needed.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

func TestUnknownSenderQueuesPairingRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	agentID, chID := uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Pairing Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO gateway_channels(id,workspace_id,agent_id,name,channel_type,config,created_by)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'pairing-test','whatsapp','{"dm_policy":"pairing"}'::jsonb,$4::uuid)`,
		chID, tn.wsID, agentID, tn.userID)

	h := NewGatewayHandler(pool, &config.Config{}, nil)
	c, err := h.repo.GetChannel(ctx, chID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	// Must go through parseGatewayConfig: it is what defaults
	// AssistantEnabled to true, and a hand-built config would bail at the
	// assistant_disabled guard before ever reaching the policy gate.
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)
	if cfg.DMPolicy != "pairing" {
		t.Fatalf("cfg.DMPolicy = %q, want pairing (the documented default)", cfg.DMPolicy)
	}

	msg := inboundMessage{
		AccountID: "default", PeerKind: "direct", PeerID: "919999999999@s.whatsapp.net",
		SenderID: "919999999999@s.whatsapp.net", SenderPhone: "+919999999999",
		Body: "hello", ProviderMessageID: "msg-1",
	}
	if accepted, reply := h.handleInbound(ctx, c, cfg, msg); accepted || reply != "" {
		t.Fatalf("unknown sender on pairing policy = (%v, %q), want (false, \"\")", accepted, reply)
	}

	var count int
	var status, sender string
	if err := pool.QueryRow(ctx,
		`SELECT count(*), coalesce(max(status),''), coalesce(max(sender_id),'')
		 FROM gateway_pairing_requests WHERE channel_id=$1::uuid`, chID).
		Scan(&count, &status, &sender); err != nil {
		t.Fatalf("query pairings: %v", err)
	}
	if count != 1 || status != "pending" || sender != msg.SenderID {
		t.Fatalf("got %d rows (status=%q sender=%q), want 1 pending row for %s", count, status, sender, msg.SenderID)
	}

	// Repeat messages must collapse onto the one pending row, or a spammer
	// grows the table without bound.
	msg.ProviderMessageID = "msg-2"
	h.handleInbound(ctx, c, cfg, msg)
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gateway_pairing_requests WHERE channel_id=$1::uuid`, chID).Scan(&count); err != nil {
		t.Fatalf("query pairings: %v", err)
	}
	if count != 1 {
		t.Fatalf("second message from same sender created %d rows, want 1", count)
	}

	// An allowlisted sender is dispatched, not queued — the policy gate still
	// discriminates instead of pairing everyone.
	allowedCfg := cfg
	allowedCfg.AllowFrom = []string{"+918888888888"}
	if !h.senderAllowed(allowedCfg, inboundMessage{PeerKind: "direct", SenderPhone: "+918888888888"}, "unmatched") {
		t.Fatal("allowlisted sender on pairing policy was blocked")
	}

	// Group traffic follows GroupPolicy ("disabled" by default), not DMPolicy.
	if got := applicablePolicy(cfg, inboundMessage{PeerKind: "group"}); got != "disabled" {
		t.Fatalf("applicablePolicy(group) = %q, want %q", got, "disabled")
	}
}

func TestApprovePairingCreatesTrustedContactRejectCreatesBlocked(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)

	agentID, chID := uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Pairing Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO gateway_channels(id,workspace_id,agent_id,name,channel_type,config,created_by)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'pairing-test-2','whatsapp','{"dm_policy":"pairing"}'::jsonb,$4::uuid)`,
		chID, tn.wsID, agentID, tn.userID)

	h := NewGatewayHandler(pool, &config.Config{}, nil)
	pairing := &domain.GatewayPairingRequest{
		WorkspaceID: tn.wsID, ChannelID: chID, AccountID: "default",
		PeerKind: "direct", PeerID: "917777777777@s.whatsapp.net", SenderID: "917777777777@s.whatsapp.net",
		Code: "ABC123", ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := h.repo.CreatePairing(ctx, pairing); err != nil {
		t.Fatalf("seed pairing: %v", err)
	}

	if err := h.createContactFromPairing(ctx, *pairing, "trusted"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	contact, err := h.repo.MatchContact(ctx, chID, "default", pairing.SenderID, "")
	if err != nil || contact == nil || contact.Role != "trusted" || !contact.AutoReplyEnabled {
		t.Fatalf("approved contact = %+v (err %v), want trusted+auto-reply", contact, err)
	}

	// A second pairing request for the SAME sender (e.g. they messaged again
	// under a stale pending row before the first was approved) must not fail
	// to create a contact just because one already exists — it flips the
	// existing row's role instead.
	pairing2 := &domain.GatewayPairingRequest{
		WorkspaceID: tn.wsID, ChannelID: chID, AccountID: "default",
		PeerKind: "direct", PeerID: pairing.PeerID, SenderID: pairing.SenderID,
		Code: "XYZ789", ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := h.createContactFromPairing(ctx, *pairing2, "blocked"); err != nil {
		t.Fatalf("reject after prior approve: %v", err)
	}
	contact, err = h.repo.MatchContact(ctx, chID, "default", pairing.SenderID, "")
	if err != nil || contact == nil || contact.Role != "blocked" {
		t.Fatalf("contact after reject = %+v (err %v), want blocked", contact, err)
	}
}
