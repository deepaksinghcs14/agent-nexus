package handler

// ChannelProvider extraction: a registry-backed replacement for the
// hardcoded `== "whatsapp" || == "http"` checks scattered across
// CreateChannel, UpdateChannel, and nexus_ai.go. These tests cover the
// registry itself and the fireScheduledMessages regression found during
// that extraction (it lacked the type guard fireReminders already has, so a
// scheduled message on an http channel tried to push through the WhatsApp
// adapter instead of failing cleanly).

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

func TestRegisteredChannelTypes(t *testing.T) {
	pool := testPool(t)
	NewGatewayHandler(pool, &config.Config{}, nil)

	got := registeredChannelTypes()
	if len(got) != 2 || got[0] != "http" || got[1] != "whatsapp" {
		t.Fatalf("registeredChannelTypes() = %v, want [http whatsapp]", got)
	}
}

func TestCreateChannelRejectsUnregisteredType(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)
	h := NewGatewayHandler(pool, &config.Config{}, nil)

	agentID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
		 VALUES($1::uuid,$2::uuid,'Provider Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID); err != nil {
		t.Fatalf("fixture insert failed: %v", err)
	}

	body := `{"name":"Bad Channel","agent_id":"` + agentID + `","channel_type":"sms"}`
	req := tn.request("POST", "/gateway/channels", "", "", body)
	w := httptest.NewRecorder()
	h.CreateChannel(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400 for an unregistered channel_type; body: %s", w.Code, w.Body.String())
	}
}

func TestFireScheduledMessagesSkipsNonWhatsAppChannel(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tn := newTenant(t, pool)
	h := NewGatewayHandler(pool, &config.Config{}, nil)

	agentID, chID, msgID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
	          VALUES($1::uuid,$2::uuid,'Scheduled Test Agent','x','anthropic','claude-test',$3::uuid)`,
		agentID, tn.wsID, tn.userID)
	mustExec(`INSERT INTO gateway_channels(id,workspace_id,agent_id,name,channel_type,config,created_by)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'http-channel','http','{}'::jsonb,$4::uuid)`,
		chID, tn.wsID, agentID, tn.userID)
	mustExec(`INSERT INTO gateway_scheduled_messages(id,workspace_id,channel_id,peer_id,message,send_at,status)
	          VALUES($1::uuid,$2::uuid,$3::uuid,'peer-1','hello',$4,'pending')`,
		msgID, tn.wsID, chID, time.Now().Add(-time.Minute))

	// If this reaches the WhatsApp adapter path it dials out to
	// h.cfg.WhatsAppAdapterURL, which is unset here — that would surface as
	// a send error, not a hang, but the point of the guard is that it must
	// never be attempted at all for an http channel.
	h.fireScheduledMessages(ctx)

	var status, lastErr string
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM gateway_scheduled_messages WHERE id=$1::uuid`, msgID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("query scheduled message: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed (http channels don't support scheduled delivery)", status)
	}
	if !strings.Contains(lastErr, "http") {
		t.Fatalf("last_error = %q, want it to mention the unsupported channel type", lastErr)
	}
}
