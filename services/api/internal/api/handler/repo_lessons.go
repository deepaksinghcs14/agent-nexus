package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo lessons: review-session verdicts are the platform's memory of what
// keeps going wrong in a repo. Each successful review appends its findings to
// repo_catalog.lessons (newest first, capped); coding-session launches inject
// them into the task so the next session doesn't repeat past mistakes.

// maxRepoLessons caps the stored lessons text — oldest entries fall off.
const maxRepoLessons = 6000

// distillReviewLessons renders a review verdict JSON into a compact lesson
// block, or "" when the verdict has no findings worth remembering.
func distillReviewLessons(ticketKey, summary string) string {
	// The review contract says "respond ONLY with JSON", but the model may
	// wrap it in prose — extract the outermost object.
	start, end := strings.Index(summary, "{"), strings.LastIndex(summary, "}")
	if start < 0 || end <= start {
		return ""
	}
	var verdict struct {
		BlockingIssues []struct {
			File       string `json:"file"`
			Issue      string `json:"issue"`
			Suggestion string `json:"suggestion"`
		} `json:"blocking_issues"`
		NonBlockingNotes []string `json:"non_blocking_notes"`
	}
	if json.Unmarshal([]byte(summary[start:end+1]), &verdict) != nil {
		return ""
	}
	var lines []string
	for _, bi := range verdict.BlockingIssues {
		line := "- " + strings.TrimSpace(bi.Issue)
		if bi.File != "" {
			line = "- [" + bi.File + "] " + strings.TrimSpace(bi.Issue)
		}
		if s := strings.TrimSpace(bi.Suggestion); s != "" {
			line += " — " + s
		}
		lines = append(lines, line)
	}
	for _, n := range verdict.NonBlockingNotes {
		if n = strings.TrimSpace(n); n != "" {
			lines = append(lines, "- "+n)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("Review findings (%s, %s):\n%s",
		ticketKey, time.Now().UTC().Format("2006-01-02"), strings.Join(lines, "\n"))
}

// appendLessons prepends a new lesson block to the existing text and truncates
// the tail to the cap, dropping the oldest partial block cleanly.
func appendLessons(existing, block string) string {
	merged := block
	if strings.TrimSpace(existing) != "" {
		merged = block + "\n\n" + existing
	}
	if len(merged) <= maxRepoLessons {
		return merged
	}
	merged = merged[:maxRepoLessons]
	// Cut at the last complete line so a truncated finding doesn't dangle.
	if i := strings.LastIndex(merged, "\n"); i > 0 {
		merged = merged[:i]
	}
	return merged
}

// recordRepoLessons persists a review session's findings against the repo the
// run's workspace has catalogued. Best-effort: a failure here must never fail
// the runner callback.
func recordRepoLessons(ctx context.Context, pool *pgxpool.Pool, runID, repo, ticketKey, summary string) {
	block := distillReviewLessons(ticketKey, summary)
	if block == "" {
		return
	}
	var workspaceID, existing string
	if err := pool.QueryRow(ctx, `
		SELECT r.workspace_id::text, rc.lessons
		FROM runs r JOIN repo_catalog rc ON rc.workspace_id = r.workspace_id AND rc.repo = $2
		WHERE r.id = $1::uuid`, runID, repo).Scan(&workspaceID, &existing); err != nil {
		slog.Warn("repo lessons: no catalog row for review callback", "run_id", runID, "repo", repo, "error", err)
		return
	}
	if _, err := pool.Exec(ctx,
		`UPDATE repo_catalog SET lessons=$3, updated_at=NOW() WHERE workspace_id=$1::uuid AND repo=$2`,
		workspaceID, repo, appendLessons(existing, block)); err != nil {
		slog.Warn("repo lessons: failed to persist review findings", "run_id", runID, "repo", repo, "error", err)
	}
}
