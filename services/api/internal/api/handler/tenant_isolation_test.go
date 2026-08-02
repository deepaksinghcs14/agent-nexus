package handler

// Cross-tenant isolation regression tests.
//
// These endpoints previously took a resource id straight from the URL and never
// checked the caller's workspace, so any authenticated user could read, modify,
// or delete another tenant's data — including provider credentials, which are
// decryptable secrets the ListModels path actually spends on a live API call.
//
// They run against real Postgres because the guards are SQL predicates: a unit
// test with a fake repository would assert nothing about the thing that broke.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
)

const testEncKey = "0123456789abcdef0123456789abcdef" // 32 bytes, AES-256

// tenant is one isolated workspace with a user, used as either attacker or victim.
type tenant struct {
	userID string
	wsID   string
}

func newTenant(t *testing.T, pool *pgxpool.Pool) tenant {
	t.Helper()
	ctx := context.Background()
	tn := tenant{userID: uuid.NewString(), wsID: uuid.NewString()}
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture insert failed: %v\n%s", err, sql)
		}
	}
	mustExec(`INSERT INTO users(id,email,password_hash) VALUES($1::uuid,$2,'x')`, tn.userID, tn.userID+"@test.local")
	mustExec(`INSERT INTO workspaces(id,name,owner_id) VALUES($1::uuid,$2,$3::uuid)`,
		tn.wsID, "tenant-"+tn.wsID[:8], tn.userID)
	return tn
}

// request builds a request carrying tenant's authenticated identity, with an
// optional chi URL param (the id these handlers read).
func (tn tenant) request(method, target, urlParamKey, urlParamVal, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ContextKeyWorkspaceID, tn.wsID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, tn.userID)
	ctx = context.WithValue(ctx, middleware.ContextKeyEmail, tn.userID+"@test.local")
	if urlParamKey != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add(urlParamKey, urlParamVal)
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return req.WithContext(ctx)
}

func seedProvider(t *testing.T, pool *pgxpool.Pool, tn tenant) string {
	t.Helper()
	id := uuid.NewString()
	enc, err := encrypt.Encrypt([]byte(testEncKey), "sk-victim-secret-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO provider_credentials(id,workspace_id,provider,display_name,encrypted_key,base_url,is_active,created_by)
		 VALUES($1::uuid,$2::uuid,'anthropic','Victim Key',$3,'',true,$4::uuid)`,
		id, tn.wsID, enc, tn.userID); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	return id
}

func TestProviderCredentialsAreWorkspaceScoped(t *testing.T) {
	pool := testPool(t)
	victim, attacker := newTenant(t, pool), newTenant(t, pool)
	providerID := seedProvider(t, pool, victim)

	h := NewProvidersHandler(pool, &config.Config{EncryptionKey: testEncKey})

	t.Run("attacker cannot update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Update(rec, attacker.request(http.MethodPut, "/providers/"+providerID,
			"id", providerID, `{"display_name":"pwned"}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404: %s", rec.Code, rec.Body.String())
		}
		var name string
		if err := pool.QueryRow(context.Background(),
			`SELECT display_name FROM provider_credentials WHERE id=$1::uuid`, providerID).Scan(&name); err != nil {
			t.Fatalf("reread provider: %v", err)
		}
		if name != "Victim Key" {
			t.Fatalf("victim's credential was modified: display_name=%q", name)
		}
	})

	t.Run("attacker cannot list models", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ListModels(rec, attacker.request(http.MethodGet, "/providers/"+providerID+"/models",
			"id", providerID, ""))
		// This path decrypts the key and calls the upstream provider — it must
		// never reach that code for a foreign credential.
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("attacker cannot delete", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Delete(rec, attacker.request(http.MethodDelete, "/providers/"+providerID, "id", providerID, ""))
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM provider_credentials WHERE id=$1::uuid`, providerID).Scan(&count); err != nil {
			t.Fatalf("recount provider: %v", err)
		}
		if count != 1 {
			t.Fatalf("victim's credential was deleted by another workspace (status %d)", rec.Code)
		}
	})

	t.Run("owner can still update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Update(rec, victim.request(http.MethodPut, "/providers/"+providerID,
			"id", providerID, `{"display_name":"renamed by owner"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("owner update got %d, want 200: %s", rec.Code, rec.Body.String())
		}
	})
}

func seedAgent(t *testing.T, pool *pgxpool.Pool, tn tenant) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO agents(id,workspace_id,name,instructions,provider,model,created_by)
		 VALUES($1::uuid,$2::uuid,'Agent','Do things.','anthropic','claude-test',$3::uuid)`,
		id, tn.wsID, tn.userID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return id
}

func seedSkill(t *testing.T, pool *pgxpool.Pool, tn tenant, name, content string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO skills(id,workspace_id,name,description,content,source,created_by)
		 VALUES($1::uuid,$2::uuid,$3,'',$4,'manual',$5::uuid)`,
		id, tn.wsID, name, content, tn.userID); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	return id
}

func TestAgentSkillsAreWorkspaceScoped(t *testing.T) {
	pool := testPool(t)
	victim, attacker := newTenant(t, pool), newTenant(t, pool)
	victimAgent := seedAgent(t, pool, victim)
	victimSkill := seedSkill(t, pool, victim, "victim-skill", "PROPRIETARY PROMPT")

	// Attach the victim's skill to the victim's own agent so there is something
	// for the attacker's read attempt to leak.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO agent_skills(agent_id,skill_id,enabled,activation_mode,order_index)
		 VALUES($1::uuid,$2::uuid,true,'always',0)`, victimAgent, victimSkill); err != nil {
		t.Fatalf("seed agent_skill: %v", err)
	}

	h := NewSkillsHandler(pool, &config.Config{EncryptionKey: testEncKey})

	t.Run("attacker cannot read another workspace's agent skills", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ListForAgent(rec, attacker.request(http.MethodGet, "/agents/"+victimAgent+"/skills",
			"id", victimAgent, ""))
		if strings.Contains(rec.Body.String(), "PROPRIETARY PROMPT") {
			t.Fatalf("skill content leaked cross-tenant: %s", rec.Body.String())
		}
		var out struct {
			Data []any `json:"data"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &out) == nil && len(out.Data) != 0 {
			t.Fatalf("expected no skills for a foreign agent, got %d", len(out.Data))
		}
	})

	t.Run("attacker cannot overwrite another workspace's agent skills", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.SetForAgent(rec, attacker.request(http.MethodPut, "/agents/"+victimAgent+"/skills",
			"id", victimAgent, `{"skills":[]}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404: %s", rec.Code, rec.Body.String())
		}
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1::uuid`, victimAgent).Scan(&count); err != nil {
			t.Fatalf("recount agent_skills: %v", err)
		}
		if count != 1 {
			t.Fatalf("victim's skill assignments were wiped by another workspace (count=%d)", count)
		}
	})

	// The audit missed this one: even with an agent guard, the skill ids in the
	// body were never validated, so a caller could pull a foreign skill's
	// content into their own agent's prompt.
	t.Run("attacker cannot attach a foreign skill to their own agent", func(t *testing.T) {
		attackerAgent := seedAgent(t, pool, attacker)
		body := `{"skills":[{"skill_id":"` + victimSkill + `","enabled":true,"activation_mode":"always","order_index":0}]}`
		rec := httptest.NewRecorder()
		h.SetForAgent(rec, attacker.request(http.MethodPut, "/agents/"+attackerAgent+"/skills",
			"id", attackerAgent, body))

		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1::uuid AND skill_id=$2::uuid`,
			attackerAgent, victimSkill).Scan(&count); err != nil {
			t.Fatalf("count cross-tenant attachment: %v", err)
		}
		if count != 0 {
			t.Fatal("a foreign workspace's skill was attached to the attacker's agent")
		}
	})

	t.Run("owner can still set skills on their own agent", func(t *testing.T) {
		body := `{"skills":[{"skill_id":"` + victimSkill + `","enabled":true,"activation_mode":"always","order_index":0}]}`
		rec := httptest.NewRecorder()
		h.SetForAgent(rec, victim.request(http.MethodPut, "/agents/"+victimAgent+"/skills",
			"id", victimAgent, body))
		if rec.Code != http.StatusOK {
			t.Fatalf("owner set got %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1::uuid AND skill_id=$2::uuid`,
			victimAgent, victimSkill).Scan(&count); err != nil {
			t.Fatalf("recount own attachment: %v", err)
		}
		if count != 1 {
			t.Fatalf("owner's own skill attachment did not persist (count=%d)", count)
		}
	})
}
