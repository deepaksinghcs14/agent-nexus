// The runner service executes autonomous Claude Code repo sessions for Agent
// Nexus. It runs as its own Railway service (one container, stdlib only):
//
//	POST /sessions  — launch (or join) a session for (ticket_key, repo);
//	                  the session clones the repo over HTTPS with GITHUB_TOKEN,
//	                  works on a fresh branch via headless Claude Code, pushes,
//	                  and POSTs a completion callback to every subscribed run.
//	                  mode=review instead checks out an existing branch
//	                  read-only, diffs it against base, and returns Claude's
//	                  review verdict in the summary (nothing pushed).
//	GET  /healthz   — liveness.
//
// Executor modes (RUNNER_EXECUTOR):
//
//	claude — real sessions: git clone + `claude -p` + git push (default)
//	stub   — simulated sessions for integration tests: sleeps STUB_DELAY_MS,
//	         then reports STUB_STATUS (default success) without touching git
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type launchRequest struct {
	RunID           string  `json:"run_id"`
	Repo            string  `json:"repo"` // owner/name
	TicketKey       string  `json:"ticket_key"`
	TaskDescription string  `json:"task_description"`
	BaseBranch      string  `json:"base_branch"`
	BudgetUSD       float64 `json:"budget_usd"`
	CallbackURL     string  `json:"callback_url"`
	CallbackSecret  string  `json:"callback_secret"`
	// Mode selects the session type: "" (or "code") runs a coding session that
	// commits and pushes a fresh branch; "review" checks out Head read-only,
	// diffs it against BaseBranch, and returns Claude's review verdict as the
	// result summary — nothing is committed or pushed.
	Mode string `json:"mode"`
	// Head is the existing branch under review (review mode only).
	Head string `json:"head"`
	// ClaudeToken, if set, authenticates the claude subprocess for this session
	// (workspace Claude account, subscription billing). Takes precedence over
	// the runner's own ANTHROPIC_API_KEY / CLAUDE_CODE_OAUTH_TOKEN env.
	ClaudeToken string `json:"claude_token"`
	// GithubToken, if set, is the workspace's GitHub token for this session's
	// clone/push. Takes precedence over the runner's own GITHUB_TOKEN env.
	GithubToken string `json:"github_token"`
}

type subscriber struct {
	RunID          string `json:"run_id"`
	CallbackURL    string `json:"callback_url"`
	CallbackSecret string `json:"callback_secret"`
}

type session struct {
	key         string // sessionKey(req): ticket_key|repo, "review:"-prefixed for review mode
	req         launchRequest
	subscribers []subscriber
	done        bool
	// cancel stops the claude subprocess for this session (via the ctx
	// runSession derives its exec.CommandContext calls from). Set once, right
	// after runSession creates it — nil until then, so a cancel request that
	// lands in that brief window is a no-op rather than a nil-pointer panic.
	cancel context.CancelFunc
}

// sessionKey is the dedup identity of a session. Review sessions get their own
// prefix so an in-flight review never absorbs a coding-session launch for the
// same (ticket, repo) — the joiner would receive a review verdict where it
// expected a coding-session result.
func sessionKey(req launchRequest) string {
	key := req.TicketKey + "|" + req.Repo
	if req.Mode == "review" {
		return "review:" + key
	}
	return key
}

// sessionJournal is the crash-recovery record persisted to WORK_DIR while a
// session is in flight. It deliberately excludes the Claude token — recovery
// only needs enough to tell every subscribed run the session died.
type sessionJournal struct {
	Key         string       `json:"key"`
	Repo        string       `json:"repo"`
	TicketKey   string       `json:"ticket_key"`
	Subscribers []subscriber `json:"subscribers"`
	StartedAt   time.Time    `json:"started_at"`
}

type server struct {
	mu       sync.Mutex
	sessions map[string]*session

	executor       string
	workDir        string
	githubToken    string
	maxTurns       int
	sessionTimeout time.Duration
	stubDelay      time.Duration
	stubStatus     string
	// callbackSecret authenticates incoming API→runner calls (cancel). Same
	// shared secret already used for the reverse direction (runner→API
	// callbacks carry it too) — one secret, not two.
	callbackSecret string
}

type result struct {
	Status  string // success | budget-exceeded | crashed
	Branch  string
	Summary string
	CostUSD float64
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	s := &server{
		sessions:       map[string]*session{},
		executor:       getEnv("RUNNER_EXECUTOR", "claude"),
		workDir:        getEnv("WORK_DIR", "/tmp/runner-sessions"),
		githubToken:    getEnv("GITHUB_TOKEN", ""),
		maxTurns:       getEnvInt("MAX_TURNS", 50),
		sessionTimeout: time.Duration(getEnvInt("SESSION_TIMEOUT_MIN", 120)) * time.Minute,
		stubDelay:      time.Duration(getEnvInt("STUB_DELAY_MS", 2000)) * time.Millisecond,
		stubStatus:     getEnv("STUB_STATUS", "success"),
		callbackSecret: getEnv("RUNNER_CALLBACK_SECRET", ""),
	}
	if err := os.MkdirAll(s.workDir, 0o755); err != nil {
		slog.Error("failed to create work dir", "error", err)
		os.Exit(1)
	}

	// RUNNER_EXTRA_CA: path to an additional trusted CA (PEM) — for local
	// installs behind a TLS-intercepting corporate proxy (e.g. Cloudflare
	// Zero Trust). The CA is appended to the system bundle and exported to
	// the git and claude subprocesses; without it, clones and API calls fail
	// with SELF_SIGNED_CERT_IN_CHAIN on such networks.
	if extraCA := getEnv("RUNNER_EXTRA_CA", ""); extraCA != "" {
		if err := trustExtraCA(extraCA, s.workDir); err != nil {
			slog.Warn("RUNNER_EXTRA_CA configured but unusable", "path", extraCA, "error", err)
		} else {
			slog.Info("extra CA trusted for git and claude subprocesses", "path", extraCA)
		}
	}

	// Sessions interrupted by a previous crash/restart: notify their runs.
	go s.recoverJournals()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions", s.handleLaunch)
	mux.HandleFunc("POST /sessions/cancel", s.handleCancel)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","executor":%q}`, s.executor)
	})

	port := getEnv("PORT", "8092")
	slog.Info("runner starting", "port", port, "executor", s.executor, "max_turns", s.maxTurns)
	if err := (&http.Server{Addr: ":" + port, Handler: mux}).ListenAndServe(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func (s *server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if s.callbackSecret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Runner-Secret")), []byte(s.callbackSecret)) != 1 {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	var req launchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.RunID == "" || req.Repo == "" || req.TicketKey == "" || req.TaskDescription == "" || req.CallbackURL == "" {
		httpErr(w, http.StatusBadRequest, "run_id, repo, ticket_key, task_description, and callback_url are required")
		return
	}
	if strings.Count(req.Repo, "/") != 1 || strings.Contains(req.Repo, "..") {
		httpErr(w, http.StatusBadRequest, "repo must be owner/name")
		return
	}
	if req.Mode == "review" && req.Head == "" {
		httpErr(w, http.StatusBadRequest, "head is required for review sessions")
		return
	}

	key := sessionKey(req)
	sub := subscriber{RunID: req.RunID, CallbackURL: req.CallbackURL, CallbackSecret: req.CallbackSecret}

	s.mu.Lock()
	existing, ok := s.sessions[key]
	if ok && !existing.done {
		// Idempotent join: an equivalent session is already running — this
		// run's callback fires when it completes.
		already := false
		for _, es := range existing.subscribers {
			if es.RunID == sub.RunID {
				already = true
				break
			}
		}
		if !already {
			existing.subscribers = append(existing.subscribers, sub)
			s.saveJournalLocked(existing)
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"session": key, "status": "already_running"})
		return
	}
	sess := &session{key: key, req: req, subscribers: []subscriber{sub}}
	s.sessions[key] = sess
	s.saveJournalLocked(sess)
	s.mu.Unlock()

	go s.runSession(sess)
	writeJSON(w, http.StatusAccepted, map[string]any{"session": key, "status": "started"})
}

// handleCancel stops the claude subprocess for a session, if one is
// currently running. Idempotent and always 200: a session that already
// finished or was never known is a no-op, not an error — the caller (the
// API's Cancel endpoint) already committed to cancelling on its side and
// this is best-effort follow-through, not a two-phase commit.
func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if s.callbackSecret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Runner-Secret")), []byte(s.callbackSecret)) != 1 {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		SessionKey string `json:"session_key"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.SessionKey == "" {
		httpErr(w, http.StatusBadRequest, "session_key is required")
		return
	}

	s.mu.Lock()
	sess, ok := s.sessions[req.SessionKey]
	cancelled := false
	if ok && !sess.done && sess.cancel != nil {
		sess.cancel()
		cancelled = true
	}
	s.mu.Unlock()

	if ok {
		slog.Info("session cancel requested", "session", req.SessionKey, "cancelled", cancelled)
	} else {
		slog.Info("session cancel requested for unknown session", "session", req.SessionKey)
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": cancelled})
}

// ── crash-recovery journal ────────────────────────────────────────────────────

func (s *server) journalPath(key string) string {
	return filepath.Join(s.workDir, "journal-"+sanitize(key)+".json")
}

// saveJournalLocked persists the session's recovery record. Caller holds s.mu.
// The journal is what lets a restarted runner tell every subscribed run that
// its in-flight session died with the process.
func (s *server) saveJournalLocked(sess *session) {
	j := sessionJournal{
		Key:         sess.key,
		Repo:        sess.req.Repo,
		TicketKey:   sess.req.TicketKey,
		Subscribers: append([]subscriber(nil), sess.subscribers...),
		StartedAt:   time.Now().UTC(),
	}
	b, err := json.Marshal(j)
	if err == nil {
		err = os.WriteFile(s.journalPath(sess.key), b, 0o600)
	}
	if err != nil {
		slog.Warn("failed to persist session journal; session will not survive a runner restart",
			"session", sess.key, "error", err)
	}
}

// recoverJournals runs once at startup: every leftover journal is a session
// that was in flight when the previous process died. Deliver a crashed
// callback to each subscribed run so the orchestrator's crash handling
// (retry once, then report) takes over instead of the run waiting forever.
func (s *server) recoverJournals() {
	entries, err := os.ReadDir(s.workDir)
	if err != nil {
		slog.Warn("journal recovery: cannot read work dir", "error", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "journal-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.workDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var j sessionJournal
		if json.Unmarshal(b, &j) != nil || len(j.Subscribers) == 0 {
			os.Remove(path) //nolint:errcheck
			continue
		}
		slog.Warn("recovering session interrupted by runner restart",
			"session", j.Key, "subscribers", len(j.Subscribers), "started_at", j.StartedAt)
		res := result{
			Status: "crashed",
			Summary: fmt.Sprintf("runner restarted while the session was executing (started %s); work in progress was lost",
				j.StartedAt.Format(time.RFC3339)),
		}
		req := launchRequest{Repo: j.Repo, TicketKey: j.TicketKey}
		for _, sub := range j.Subscribers {
			s.deliverCallback(sub, j.Key, req, res)
		}
		os.Remove(path) //nolint:errcheck
	}
}

func (s *server) runSession(sess *session) {
	slog.Info("session started", "session", sess.key, "executor", s.executor, "run_id", sess.req.RunID)
	ctx, cancel := context.WithTimeout(context.Background(), s.sessionTimeout)
	defer cancel()
	s.mu.Lock()
	sess.cancel = cancel
	s.mu.Unlock()

	var res result
	if s.executor == "stub" {
		time.Sleep(s.stubDelay)
		res = result{
			Status: s.stubStatus,
			Branch: "nexus/" + sess.req.TicketKey,
			Summary: "SIMULATED SESSION (runner is in stub mode — no code was written, no branch was pushed). " +
				"This validates pipeline plumbing only. Set RUNNER_EXECUTOR=claude on the runner for real coding sessions. " +
				"Simulated work order: " + sess.key,
			CostUSD: 0.01,
		}
		if sess.req.Mode == "review" {
			res.Branch = sess.req.Head
			res.Summary = `{"verdict":"approve","blocking_issues":[],"non_blocking_notes":["SIMULATED REVIEW (runner is in stub mode — no diff was inspected)"],"pr_description_notes":"Simulated review — stub mode validates pipeline plumbing only."}`
		}
	} else if sess.req.Mode == "review" {
		res = s.runReviewSession(ctx, sess)
	} else {
		res = s.runClaudeSession(ctx, sess)
	}

	s.mu.Lock()
	sess.done = true
	subs := append([]subscriber(nil), sess.subscribers...)
	s.mu.Unlock()

	slog.Info("session finished", "session", sess.key, "status", res.Status, "cost_usd", res.CostUSD)
	for _, sub := range subs {
		s.deliverCallback(sub, sess.key, sess.req, res)
	}
	// The session reached a terminal state and every callback was attempted —
	// it no longer needs crash recovery. (Duplicate crashed callbacks after an
	// ill-timed restart are acknowledged and ignored by the API.)
	os.Remove(s.journalPath(sess.key)) //nolint:errcheck
}

// codeQualityPrompt is appended to the coding session's system prompt to keep
// autonomous sessions from over-engineering — the recurring quality problem
// with unsupervised runs is too much code, not too little.
const codeQualityPrompt = `Code quality rules for this session:
- Write the shortest working diff. Before writing anything new, look for an existing helper, util, or pattern in this repo and reuse it — re-implementing what already exists a few files over is the most common failure.
- Prefer the standard library over new dependencies; never add a dependency for what a few lines can do.
- No speculative abstractions: no interface with one implementation, no config for a value that never changes, no scaffolding "for later".
- Fix root causes, not symptoms: before editing a function, check its callers — one guard in the shared path beats a patch in each caller.
- Match the surrounding code's idiom, naming, and comment density exactly.
- Comments only for constraints the code cannot show — never to narrate what a line does.
- For non-trivial new logic, leave one minimal runnable check (a small test or assert) in the repo's existing test style. No new test frameworks.
- If the task is ambiguous, implement the smallest reasonable interpretation and note the ambiguity in your final summary instead of building every variant.`

// pushTimeout bounds the post-session salvage push. Deliberately not an env
// knob: it only ever has to cover one push of a shallow branch, on its own
// fresh context — the session ctx is already expired on exactly the path
// that matters (a SESSION_TIMEOUT_MIN kill via exec.CommandContext), so
// reusing it would kill git the instant it started.
const pushTimeout = 5 * time.Minute

// salvagePush commits whatever the session left uncommitted and pushes the
// branch, reporting whether it actually reached the remote. Runs regardless
// of how the session ended — a killed or crashed session's partial work is
// the whole point of the fallback, and the caller's deferred os.RemoveAll is
// about to delete the only copy.
func salvagePush(dir, branch, baseSHA, githubToken string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()

	// The prompt told the session to commit everything it changed, so a dirty
	// tree here means it was killed mid-edit or disobeyed — either way the
	// diff is the deliverable and the alternative is losing it outright.
	// ponytail: sweeps untracked scratch files too; .gitignore is the only
	// filter, and the review session is what catches the rest.
	if st, err := runCmd(ctx, dir, nil, "git", "status", "--porcelain"); err != nil {
		slog.Warn("salvage: git status failed, skipping uncommitted-work sweep", "branch", branch, "error", err)
	} else if strings.TrimSpace(st) != "" {
		if out, err := runCmd(ctx, dir, nil, "git", "add", "-A"); err != nil {
			slog.Warn("salvage: git add failed", "branch", branch, "error", err, "detail", truncate(out, 300))
		} else if out, err := runCmd(ctx, dir, nil, "git", "commit", "-m", "wip: session ended before it could commit"); err != nil {
			slog.Warn("salvage: wip commit failed", "branch", branch, "error", err, "detail", truncate(out, 300))
		}
	}

	head, err := runCmd(ctx, dir, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		slog.Error("salvage: cannot read HEAD, nothing pushed", "branch", branch, "error", err, "detail", truncate(head, 300))
		return false
	}
	if strings.TrimSpace(head) == baseSHA {
		return false // nothing was committed
	}
	// --force: the branch is derived from the ticket key, so the
	// orchestrator's "crashed → retry once" relaunches into this same ref —
	// without --force the retry's own (successful) push is rejected
	// non-fast-forward and reports as lost.
	// ponytail: safe while nexus/<ticket> is runner-owned; switch to
	// --force-with-lease (needs a fetch of the remote ref first) if humans
	// ever push to these branches directly.
	if out, err := runCmd(ctx, dir, nil, "git", "push", "--force", "-u", "origin", branch); err != nil {
		slog.Error("salvage: push failed, session work stays only in the clone about to be deleted",
			"branch", branch, "error", err, "detail", redactToken(out, githubToken))
		return false
	}
	return true
}

// runClaudeSession clones the repo, runs headless Claude Code on a fresh
// branch, pushes it, and classifies the outcome.
func (s *server) runClaudeSession(ctx context.Context, sess *session) result {
	req := sess.req
	onProgress := s.newProgressReporter(sess)
	crash := func(stage string, err error, detail string) result {
		slog.Error("session stage failed", "ticket", req.TicketKey, "repo", req.Repo, "stage", stage, "error", err, "detail", detail)
		return result{Status: "crashed", Summary: fmt.Sprintf("%s failed: %v: %s", stage, err, truncate(detail, 500))}
	}
	// Workspace token from the launch request wins; runner env is the
	// single-tenant fallback.
	githubToken := req.GithubToken
	if githubToken == "" {
		githubToken = s.githubToken
	}
	if githubToken == "" {
		return crash("setup", fmt.Errorf("no GitHub token: set one in Settings → Claude Code, or GITHUB_TOKEN on the runner"), "")
	}

	dir := filepath.Join(s.workDir, sanitize(req.TicketKey+"-"+strings.ReplaceAll(req.Repo, "/", "-")+"-"+strconv.FormatInt(time.Now().UnixNano(), 36)))
	defer os.RemoveAll(dir)

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", githubToken, req.Repo)
	branch := "nexus/" + sanitize(req.TicketKey)

	cloneArgs := []string{"clone", "--depth", "50"}
	if req.BaseBranch != "" {
		cloneArgs = append(cloneArgs, "--branch", req.BaseBranch)
	}
	cloneArgs = append(cloneArgs, cloneURL, dir)
	if out, err := runCmd(ctx, "", nil, "git", cloneArgs...); err != nil {
		return crash("clone", err, redactToken(out, githubToken))
	}
	if out, err := runCmd(ctx, dir, nil, "git", "checkout", "-B", branch); err != nil {
		return crash("branch", err, out)
	}
	// Baseline for "did this session commit anything": an empty branch pushed
	// to the remote would falsely signal to the orchestrator that work landed.
	baseSHA, err := runCmd(ctx, dir, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return crash("branch", err, baseSHA)
	}
	baseSHA = strings.TrimSpace(baseSHA)
	for _, kv := range [][2]string{{"user.name", "Agent Nexus"}, {"user.email", "nexus@bureau.id"}} {
		runCmd(ctx, dir, nil, "git", "config", kv[0], kv[1]) //nolint:errcheck
	}

	prompt := fmt.Sprintf(
		"You are working on Jira ticket %s in the repository %s.\n\n"+
			"Task:\n%s\n\n"+
			"Make the required changes, keep commits small with clear messages, and commit everything you change. "+
			"Do not push — the harness pushes your branch when you finish.",
		req.TicketKey, req.Repo, req.TaskDescription)

	claudeOut, claudeErr := runClaudeStreaming(ctx, dir, claudeEnv(req.ClaudeToken), onProgress,
		"claude", "-p", prompt,
		"--append-system-prompt", codeQualityPrompt,
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", strconv.Itoa(s.maxTurns),
		"--dangerously-skip-permissions")

	// Parse the CLI's JSON result regardless of exit code — error states
	// (max turns, crashes) still emit a parseable payload in most cases.
	var cli struct {
		Subtype      string  `json:"subtype"`
		IsError      bool    `json:"is_error"`
		Result       string  `json:"result"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	parseErr := json.Unmarshal([]byte(lastJSONObject(claudeOut)), &cli)

	// Push whatever was committed regardless of how the session ended — even
	// a killed or crashed session's partial work is the whole point of the
	// fallback, and the deferred RemoveAll above is about to delete the only
	// copy. Must run before the crash return below, not after: a killed
	// session (SESSION_TIMEOUT_MIN) emits no parseable JSON, and that early
	// return used to skip the push entirely.
	pushed := salvagePush(dir, branch, baseSHA, githubToken)

	if claudeErr != nil && parseErr != nil {
		res := crash("claude", claudeErr, truncate(claudeOut, 800))
		if pushed {
			res.Branch = branch
			res.Summary = "[session did not finish — its partial, INCOMPLETE work was force-pushed to " + branch +
				" for inspection; do not open a PR from it. A retry for this ticket restarts from the base branch and overwrites it] " + res.Summary
		}
		return res
	}

	res := result{Branch: branch, Summary: truncate(cli.Result, 4000), CostUSD: cli.TotalCostUSD}
	switch {
	case cli.Subtype == "error_max_turns",
		req.BudgetUSD > 0 && cli.TotalCostUSD > req.BudgetUSD:
		res.Status = "budget-exceeded"
	case cli.IsError || claudeErr != nil:
		res.Status = "crashed"
	default:
		res.Status = "success"
	}
	if !pushed {
		res.Branch = ""
		res.Summary = "[no branch on the remote — the session committed nothing, or the push failed] " + res.Summary
	}
	return res
}

// runReviewSession checks out an existing branch, diffs it against base, and
// runs headless Claude Code as a read-only reviewer over the checkout. Nothing
// is committed or pushed; the reviewer's verdict JSON is the result summary.
func (s *server) runReviewSession(ctx context.Context, sess *session) result {
	req := sess.req
	onProgress := s.newProgressReporter(sess)
	crash := func(stage string, err error, detail string) result {
		slog.Error("review stage failed", "ticket", req.TicketKey, "repo", req.Repo, "stage", stage, "error", err, "detail", detail)
		return result{Status: "crashed", Summary: fmt.Sprintf("%s failed: %v: %s", stage, err, truncate(detail, 500))}
	}
	githubToken := req.GithubToken
	if githubToken == "" {
		githubToken = s.githubToken
	}
	if githubToken == "" {
		return crash("setup", fmt.Errorf("no GitHub token: set one in Settings → Claude Code, or GITHUB_TOKEN on the runner"), "")
	}

	dir := filepath.Join(s.workDir, sanitize("review-"+req.TicketKey+"-"+strings.ReplaceAll(req.Repo, "/", "-")+"-"+strconv.FormatInt(time.Now().UnixNano(), 36)))
	defer os.RemoveAll(dir)

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", githubToken, req.Repo)
	base := req.BaseBranch
	if base == "" {
		base = "main"
	}

	// Bounded-depth clone of base plus a fetch of head: enough history for a
	// three-dot diff in the common case without paying for full repo history.
	if out, err := runCmd(ctx, "", nil, "git", "clone", "--no-checkout", "--depth", "200", "--branch", base, cloneURL, dir); err != nil {
		return crash("clone", err, redactToken(out, githubToken))
	}
	if out, err := runCmd(ctx, dir, nil, "git", "fetch", "--depth", "200", "origin", req.Head+":refs/remotes/origin/"+req.Head); err != nil {
		return crash("fetch", err, redactToken(out, githubToken))
	}
	// Materialize the working tree at the head commit so the reviewer can read
	// files and run read-only checks (the --no-checkout clone left it empty).
	if out, err := runCmd(ctx, dir, nil, "git", "checkout", "-B", req.Head, "refs/remotes/origin/"+req.Head); err != nil {
		return crash("checkout", err, out)
	}

	diff, diffErr := runCmd(ctx, dir, nil, "git", "diff", "origin/"+base+"...origin/"+req.Head)
	if diffErr != nil {
		// Shallow clones can lack a merge base for the three-dot diff — deepen
		// once (while origin still has the credential) and retry.
		if out, err := runCmd(ctx, dir, nil, "git", "fetch", "--unshallow", "origin"); err != nil {
			return crash("unshallow", err, redactToken(out, githubToken))
		}
		if diff, diffErr = runCmd(ctx, dir, nil, "git", "diff", "origin/"+base+"...origin/"+req.Head); diffErr != nil {
			return crash("diff", diffErr, diff)
		}
	}
	const diffBudget = 60000
	if len(diff) > diffBudget {
		diff = diff[:diffBudget] + "\n…[diff truncated — inspect the checkout for the full changes]"
	}

	// The review session must not be able to write to the remote: strip the
	// token from origin before the model gets a shell. No credential helper is
	// configured on this container, so any push attempted from inside the
	// session fails instead of silently mutating the branch under review.
	if out, err := runCmd(ctx, dir, nil, "git", "remote", "set-url", "origin", "https://github.com/"+req.Repo+".git"); err != nil {
		return crash("sanitize-remote", err, out)
	}

	prompt := fmt.Sprintf(
		"You are reviewing branch %s of repository %s (base: %s) for ticket %s.\n\n"+
			"%s\n\n"+
			"This is a READ-ONLY review session: do not modify, commit, or push anything. "+
			"The repository is checked out at the head branch — you may read files and run "+
			"tests, linters, and other read-only inspection commands.\n\n"+
			"Unified diff (%s...%s):\n%s",
		req.Head, req.Repo, base, req.TicketKey,
		req.TaskDescription,
		base, req.Head, diff)

	claudeOut, claudeErr := runClaudeStreaming(ctx, dir, claudeEnv(req.ClaudeToken), onProgress,
		"claude", "-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", strconv.Itoa(s.maxTurns),
		"--dangerously-skip-permissions")

	var cli struct {
		Subtype      string  `json:"subtype"`
		IsError      bool    `json:"is_error"`
		Result       string  `json:"result"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	parseErr := json.Unmarshal([]byte(lastJSONObject(claudeOut)), &cli)
	if claudeErr != nil && parseErr != nil {
		return crash("claude", claudeErr, truncate(claudeOut, 800))
	}

	// The verdict JSON is the whole deliverable — allow a much larger summary
	// than coding sessions so a multi-issue verdict is never cut mid-structure.
	res := result{Branch: req.Head, Summary: truncate(cli.Result, 12000), CostUSD: cli.TotalCostUSD}
	switch {
	case cli.Subtype == "error_max_turns",
		req.BudgetUSD > 0 && cli.TotalCostUSD > req.BudgetUSD:
		res.Status = "budget-exceeded"
	case cli.IsError || claudeErr != nil:
		res.Status = "crashed"
	default:
		res.Status = "success"
	}
	return res
}

// claudeEnv returns the subprocess environment for the claude CLI. A
// per-request Claude account token (subscription billing) wins over any static
// credentials on the service; the env keys are stripped first so the CLI
// cannot pick the wrong one.
func claudeEnv(claudeToken string) []string {
	env := os.Environ()
	if claudeToken == "" {
		return env
	}
	filtered := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") || strings.HasPrefix(kv, "CLAUDE_CODE_OAUTH_TOKEN=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return append(filtered, "CLAUDE_CODE_OAUTH_TOKEN="+claudeToken)
}

// deliverCallback POSTs the session result to one subscriber, retrying with
// backoff. Agent Nexus treats duplicate deliveries as no-ops, so retrying on
// ambiguous failures is safe.
func (s *server) deliverCallback(sub subscriber, key string, req launchRequest, res result) {
	payload, _ := json.Marshal(map[string]any{
		"run_id":     sub.RunID,
		"session_id": key,
		"status":     res.Status,
		"mode":       req.Mode,
		"repo":       req.Repo,
		"ticket_key": req.TicketKey,
		"branch":     res.Branch,
		"summary":    res.Summary,
		"cost_usd":   res.CostUSD,
	})
	backoff := 2 * time.Second
	for attempt := 1; attempt <= 6; attempt++ {
		httpReq, err := http.NewRequest(http.MethodPost, sub.CallbackURL, bytes.NewReader(payload))
		if err != nil {
			slog.Error("callback request build failed", "run_id", sub.RunID, "error", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Runner-Secret", sub.CallbackSecret)
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpReq)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				slog.Info("callback delivered", "run_id", sub.RunID, "attempt", attempt)
				return
			}
			// 4xx other than 408/429 will not succeed on retry.
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
				slog.Error("callback rejected", "run_id", sub.RunID, "status", resp.StatusCode)
				return
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		slog.Warn("callback attempt failed", "run_id", sub.RunID, "attempt", attempt, "error", err)
		time.Sleep(backoff)
		backoff *= 2
	}
	slog.Error("callback delivery exhausted retries", "run_id", sub.RunID)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// trustExtraCA writes a combined bundle (system CAs + the extra CA) into the
// work dir and points git and node/claude at it via env vars inherited by
// every subprocess this service spawns.
func trustExtraCA(extraPath, workDir string) error {
	extra, err := os.ReadFile(extraPath)
	if err != nil {
		return err
	}
	combined := filepath.Join(workDir, "ca-bundle.crt")
	system, _ := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
	if err := os.WriteFile(combined, append(append(system, '\n'), extra...), 0o644); err != nil {
		return err
	}
	// node adds NODE_EXTRA_CA_CERTS on top of its bundled roots; git (libcurl)
	// replaces its bundle, hence the combined file.
	os.Setenv("NODE_EXTRA_CA_CERTS", extraPath) //nolint:errcheck
	os.Setenv("GIT_SSL_CAINFO", combined)       //nolint:errcheck
	return nil
}

func runCmd(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// runClaudeStreaming runs the claude CLI with --output-format stream-json,
// invoking onLine for each NDJSON line as it arrives (for live progress
// reporting) while still accumulating the full stdout so the final "result"
// object can be parsed exactly as lastJSONObject already does — unlike
// runCmd (used for git and everything else), this needs line-by-line
// visibility, not just the final buffer.
func runClaudeStreaming(ctx context.Context, dir string, env []string, onLine func(line string), name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var out bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	// A stream-json line can embed a large tool input/output; the default
	// 64KB scanner limit is too easy to hit. 1MB matches the ceiling nothing
	// in this codebase's tool outputs is expected to exceed.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		out.WriteString(line)
		out.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	out.Write(stderrBuf.Bytes())

	if waitErr != nil {
		return out.String(), waitErr
	}
	return out.String(), scanErr
}

// summarizeStreamEvent extracts a coarse "what's happening" string from one
// stream-json line, or "" if the line has nothing progress-worthy — init,
// hook, and rate-limit events are skipped, and the final result line gets
// its own terminal callback rather than a progress update.
func summarizeStreamEvent(line string) string {
	var evt struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &evt) != nil || evt.Type != "assistant" {
		return ""
	}
	for _, block := range evt.Message.Content {
		if block.Type == "tool_use" && block.Name != "" {
			return "using " + block.Name
		}
	}
	if len(evt.Message.Content) > 0 {
		return "writing a response"
	}
	return ""
}

// progressInterval bounds how often a session reports progress. A coarse
// "still alive and roughly where it is" signal, not a transcript — no
// caller needs one POST per tool call on a session that can run for hours.
const progressInterval = 5 * time.Second

// newProgressReporter returns a callback for runClaudeStreaming's onLine
// that debounces to at most one report per progressInterval. Single call
// path (the scanner loop in runClaudeStreaming runs on one goroutine), so no
// lock is needed around `last`.
func (s *server) newProgressReporter(sess *session) func(line string) {
	var last time.Time
	return func(line string) {
		summary := summarizeStreamEvent(line)
		if summary == "" || time.Since(last) < progressInterval {
			return
		}
		last = time.Now()
		s.reportProgress(sess, summary)
	}
}

// reportProgress posts a progress update to every subscriber of sess, the
// same fan-out deliverCallback uses for the terminal result. Best-effort and
// fire-and-forget: a progress update is never worth blocking or failing the
// session for, and the stale-session watchdog is the real backstop if the
// API never hears from the runner again.
func (s *server) reportProgress(sess *session, summary string) {
	s.mu.Lock()
	subs := append([]subscriber(nil), sess.subscribers...)
	s.mu.Unlock()
	for _, sub := range subs {
		progressURL := strings.TrimSuffix(sub.CallbackURL, "/internal/sessions/callback") + "/internal/sessions/progress"
		payload, _ := json.Marshal(map[string]any{"run_id": sub.RunID, "summary": summary})
		go func(url, secret, runID string) {
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Runner-Secret", secret)
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				slog.Warn("progress report failed", "run_id", runID, "error", err)
				return
			}
			resp.Body.Close()
		}(progressURL, sub.CallbackSecret, sub.RunID)
	}
}

// lastJSONObject extracts the last top-level {...} object from mixed output.
func lastJSONObject(s string) string {
	end := strings.LastIndex(s, "}")
	if end == -1 {
		return ""
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return s[i : end+1]
			}
		}
	}
	return ""
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func redactToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
