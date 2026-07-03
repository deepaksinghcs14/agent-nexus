package handler

import (
	"context"
	"encoding/json"

	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runWaitState is the loop state a run goroutine snapshots to run_wait_states
// before blocking on an approval decision. It captures everything that lives
// only on the goroutine's stack — enough to re-enter the tool-calling loop at
// the exact pending call after a process restart or a parked wait.
//
// Everything else (agent config, tool definitions, skills, provider
// credentials) is reloaded from the database on resume.
type runWaitState struct {
	Version int `json:"version"`
	// WaitType is 'approval' (blocked at the approval gate; resume applies a
	// decision AT the pending call) or 'session' (blocked inside a session
	// tool; resume injects the pending call's RESULT and continues after it).
	WaitType         string              `json:"wait_type,omitempty"`
	WorkspaceID      string              `json:"workspace_id"`
	UserID           string              `json:"user_id"`
	AgentID          string              `json:"agent_id"`
	ConversationID   string              `json:"conversation_id"`
	Input            string              `json:"input"`
	Messages         []provider.Message  `json:"messages"`
	StableSystem     string              `json:"stable_system,omitempty"`
	PendingCalls     []provider.ToolCall `json:"pending_calls"`
	CallIndex        int                 `json:"call_index"`
	StepCount        int                 `json:"step_count"`
	TotalInput       int                 `json:"total_input"`
	TotalOutput      int                 `json:"total_output"`
	ActionLog        []string            `json:"action_log,omitempty"`
	MemorySaveCalled bool                `json:"memory_save_called,omitempty"`
	RequestedTools   []string            `json:"requested_tools,omitempty"`
	ActiveSkills     []string            `json:"active_skills,omitempty"`
	ActiveMemoryIDs  []string            `json:"active_memory_ids,omitempty"`
}

// saveWaitState upserts the snapshot for runID. Called before the goroutine
// blocks on an external decision/callback, and again (with an advanced
// CallIndex) if a resumed run hits another wait.
func saveWaitState(ctx context.Context, pool *pgxpool.Pool, runID string, st *runWaitState) error {
	if st.WaitType == "" {
		st.WaitType = "approval"
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO run_wait_states(run_id, wait_type, state) VALUES($1::uuid, $2, $3::jsonb)
		 ON CONFLICT (run_id) DO UPDATE SET wait_type=$2, state=$3::jsonb, created_at=NOW()`,
		runID, st.WaitType, b)
	return err
}

// claimWaitState atomically removes and returns the snapshot for runID,
// scoped to the given wait type so an approval decision can never claim a
// session wait (or vice versa). Returns (nil, nil) when no matching snapshot
// exists — either the run was never parked or another caller already claimed
// it. The delete-returning claim is what prevents two callers from resuming
// the same run twice.
func claimWaitState(ctx context.Context, pool *pgxpool.Pool, runID, waitType string) (*runWaitState, error) {
	var b []byte
	err := pool.QueryRow(ctx,
		`DELETE FROM run_wait_states WHERE run_id=$1::uuid AND wait_type=$2 RETURNING state`, runID, waitType).Scan(&b)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st runWaitState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// deleteWaitState removes the snapshot without reading it. Called when the
// in-process wait is resolved through the registry channel (fast path) and on
// run cancellation.
func deleteWaitState(ctx context.Context, pool *pgxpool.Pool, runID string) {
	pool.Exec(ctx, `DELETE FROM run_wait_states WHERE run_id=$1::uuid`, runID) //nolint:errcheck
}

// keysOf returns the true keys of a set-map as a sorted-independent slice for
// snapshot serialization.
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}

// setOf rebuilds a set-map from a snapshot slice.
func setOf(keys []string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}
