package handler

// Inbound message dedup used to be a racy check-then-act: HasEventForProviderMessage
// (SELECT EXISTS) followed by an unconditional fall-through, regardless of whether the
// later gateway_events INSERT actually won the ON CONFLICT DO NOTHING race. Two
// near-simultaneous webhook deliveries of the same message could both pass the check
// and both run the rest of handleInbound — a *sequential* pair of calls can't
// reproduce that (by the time call 2's SELECT runs, call 1's INSERT has already
// committed), so this test fires genuinely concurrent calls to exercise the actual
// race window between check and insert.

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

func TestHandleInboundClaimsMessageAtomicallyUnderConcurrency(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)
	h := NewGatewayHandler(pool, &config.Config{}, nil)

	agentID, chID := uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Dedup Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO gateway_channels(id,workspace_id,agent_id,name,channel_type,config,created_by)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'dedup-test','whatsapp','{"dm_policy":"pairing"}'::jsonb,$4::uuid)`,
		chID, tn.wsID, agentID, tn.userID)

	c, err := h.repo.GetChannel(ctx, chID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	cfg := h.parseGatewayConfig(c.Config, c.ChannelType)

	// An unmatched sender under the default "pairing" policy is rejected by
	// senderAllowed and never reaches dispatchGatewayRun — deliberately, so
	// this test doesn't need a working *InvokeHandler (h.invoke is nil
	// here). What it DOES exercise is everything before that rejection:
	// the dedup claim, contact resolution, and the requestPairing +
	// sender_ignored side effects — exactly the code a duplicate delivery
	// would re-run if the claim weren't atomic.
	msg := inboundMessage{
		AccountID: "default", PeerKind: "direct", PeerID: "919999999999@s.whatsapp.net",
		SenderID: "919999999999@s.whatsapp.net", SenderPhone: "+919999999999",
		Body: "hello", ProviderMessageID: "wamid.duplicate-test-1",
	}

	// Simulates N near-simultaneous redeliveries of the same webhook event
	// (retry storm, adapter reconnect replay).
	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			h.handleInbound(ctx, c, cfg, msg)
		}()
	}
	wg.Wait()

	var messageReceivedCount, senderIgnoredCount, pairingCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gateway_events WHERE channel_id=$1::uuid AND event_type='message_received' AND provider_message_id=$2`,
		c.ID, msg.ProviderMessageID).Scan(&messageReceivedCount); err != nil {
		t.Fatalf("count message_received events: %v", err)
	}
	if messageReceivedCount != 1 {
		t.Fatalf("message_received events = %d, want 1 across %d concurrent calls", messageReceivedCount, concurrency)
	}

	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gateway_events WHERE channel_id=$1::uuid AND event_type='sender_ignored'`,
		c.ID).Scan(&senderIgnoredCount); err != nil {
		t.Fatalf("count sender_ignored events: %v", err)
	}
	if senderIgnoredCount != 1 {
		t.Fatalf("sender_ignored events = %d, want 1 — every call but the one that won the claim should have short-circuited before ever re-evaluating the policy gate", senderIgnoredCount)
	}

	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gateway_pairing_requests WHERE channel_id=$1::uuid`,
		c.ID).Scan(&pairingCount); err != nil {
		t.Fatalf("count pairing requests: %v", err)
	}
	if pairingCount != 1 {
		t.Fatalf("pairing requests = %d, want 1 — concurrent redeliveries must not queue duplicate pairing requests", pairingCount)
	}
}
