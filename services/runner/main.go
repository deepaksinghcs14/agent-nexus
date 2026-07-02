// The runner service executes autonomous Claude Code repo sessions for Agent
// Nexus. It runs as its own Railway service (one container, stdlib only):
//
//	POST /sessions  — launch (or join) a session for (ticket_key, repo);
//	                  the session clones the repo over HTTPS with GITHUB_TOKEN,
//	                  works on a fresh branch via headless Claude Code, pushes,
//	                  and POSTs a completion callback to every subscribed run.
//	GET  /healthz   — liveness.
//
// Executor modes (RUNNER_EXECUTOR):
//
//	claude — real sessions: git clone + `claude -p` + git push (default)
//	stub   — simulated sessions for integration tests: sleeps STUB_DELAY_MS,
//	         then reports STUB_STATUS (default success) without touching git
package main

import (
	"bytes"
	"context"
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
	key         string // ticket_key|repo
	req         launchRequest
	subscribers []subscriber
	done        bool
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
	}
	if err := os.MkdirAll(s.workDir, 0o755); err != nil {
		slog.Error("failed to create work dir", "error", err)
		os.Exit(1)
	}

	// Sessions interrupted by a previous crash/restart: notify their runs.
	go s.recoverJournals()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions", s.handleLaunch)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	port := getEnv("PORT", "8092")
	slog.Info("runner starting", "port", port, "executor", s.executor, "max_turns", s.maxTurns)
	if err := (&http.Server{Addr: ":" + port, Handler: mux}).ListenAndServe(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func (s *server) handleLaunch(w http.ResponseWriter, r *http.Request) {
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

	key := req.TicketKey + "|" + req.Repo
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
			s.deliverCallback(sub, req, res)
		}
		os.Remove(path) //nolint:errcheck
	}
}

func (s *server) runSession(sess *session) {
	slog.Info("session started", "session", sess.key, "executor", s.executor, "run_id", sess.req.RunID)
	ctx, cancel := context.WithTimeout(context.Background(), s.sessionTimeout)
	defer cancel()

	var res result
	if s.executor == "stub" {
		time.Sleep(s.stubDelay)
		res = result{
			Status:  s.stubStatus,
			Branch:  "nexus/" + sess.req.TicketKey,
			Summary: "stub session completed for " + sess.key,
			CostUSD: 0.01,
		}
	} else {
		res = s.runClaudeSession(ctx, sess.req)
	}

	s.mu.Lock()
	sess.done = true
	subs := append([]subscriber(nil), sess.subscribers...)
	s.mu.Unlock()

	slog.Info("session finished", "session", sess.key, "status", res.Status, "cost_usd", res.CostUSD)
	for _, sub := range subs {
		s.deliverCallback(sub, sess.req, res)
	}
	// The session reached a terminal state and every callback was attempted —
	// it no longer needs crash recovery. (Duplicate crashed callbacks after an
	// ill-timed restart are acknowledged and ignored by the API.)
	os.Remove(s.journalPath(sess.key)) //nolint:errcheck
}

// runClaudeSession clones the repo, runs headless Claude Code on a fresh
// branch, pushes it, and classifies the outcome.
func (s *server) runClaudeSession(ctx context.Context, req launchRequest) result {
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
	for _, kv := range [][2]string{{"user.name", "Agent Nexus"}, {"user.email", "nexus@bureau.id"}} {
		runCmd(ctx, dir, nil, "git", "config", kv[0], kv[1]) //nolint:errcheck
	}

	prompt := fmt.Sprintf(
		"You are working on Jira ticket %s in the repository %s.\n\n"+
			"Task:\n%s\n\n"+
			"Make the required changes, keep commits small with clear messages, and commit everything you change. "+
			"Do not push — the harness pushes your branch when you finish.",
		req.TicketKey, req.Repo, req.TaskDescription)

	// Per-request Claude account token (subscription billing) wins over any
	// static credentials on the service; strip the env keys so the CLI cannot
	// pick the wrong one.
	env := os.Environ()
	if req.ClaudeToken != "" {
		filtered := env[:0]
		for _, kv := range env {
			if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") || strings.HasPrefix(kv, "CLAUDE_CODE_OAUTH_TOKEN=") {
				continue
			}
			filtered = append(filtered, kv)
		}
		env = append(filtered, "CLAUDE_CODE_OAUTH_TOKEN="+req.ClaudeToken)
	}

	claudeOut, claudeErr := runCmd(ctx, dir, env,
		"claude", "-p", prompt,
		"--output-format", "json",
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
	if claudeErr != nil && parseErr != nil {
		return crash("claude", claudeErr, truncate(claudeOut, 800))
	}

	// Push whatever was committed, even on budget-exceeded — partial progress
	// on the branch is the whole point of the fallback.
	pushed := false
	if out, err := runCmd(ctx, dir, nil, "git", "push", "-u", "origin", branch); err != nil {
		slog.Warn("push failed", "ticket", req.TicketKey, "error", err, "detail", redactToken(out, githubToken))
	} else {
		pushed = true
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
		res.Summary = "[branch push failed — changes not on remote] " + res.Summary
	}
	return res
}

// deliverCallback POSTs the session result to one subscriber, retrying with
// backoff. Agent Nexus treats duplicate deliveries as no-ops, so retrying on
// ambiguous failures is safe.
func (s *server) deliverCallback(sub subscriber, req launchRequest, res result) {
	payload, _ := json.Marshal(map[string]any{
		"run_id":     sub.RunID,
		"session_id": req.TicketKey + "|" + req.Repo,
		"status":     res.Status,
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
