package native

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Jira REST tools for the Jira→PR pipeline, using API-token auth for orgs
// whose Atlassian site blocks MCP OAuth. Cloud authenticates with
// email + API token (Basic); Data Center with a PAT and email left empty
// (Bearer). Credential resolution is workspace-first (Settings → Claude
// Code), with instance-level JIRA_* envs as a single-tenant fallback.

var jiraHTTP = &http.Client{Timeout: 30 * time.Second}

type jiraCreds struct {
	BaseURL string
	Email   string
	Token   string
}

func (c jiraCreds) ok() bool { return c.BaseURL != "" && c.Token != "" }

// jiraCredsFor resolves the effective Jira credentials for a workspace.
func jiraCredsFor(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, workspaceID string) jiraCreds {
	if pool != nil && workspaceID != "" {
		var baseURL, email, encToken *string
		if err := pool.QueryRow(ctx,
			`SELECT jira_base_url, jira_email, jira_api_token FROM runner_credentials WHERE workspace_id=$1::uuid`,
			workspaceID).Scan(&baseURL, &email, &encToken); err == nil &&
			baseURL != nil && *baseURL != "" && encToken != nil && *encToken != "" {
			if token, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), *encToken); err == nil {
				c := jiraCreds{BaseURL: *baseURL, Token: token}
				if email != nil {
					c.Email = *email
				}
				return c
			}
		}
	}
	return jiraCreds{BaseURL: cfg.JiraBaseURL, Email: cfg.JiraEmail, Token: cfg.JiraAPIToken}
}

var errNoJiraCreds = fmt.Errorf("no Jira credentials: set base URL + API token in Settings → Claude Code, or JIRA_BASE_URL/JIRA_EMAIL/JIRA_API_TOKEN on the API")

func jiraRequest(ctx context.Context, creds jiraCreds, method, path string, body any) (int, []byte, error) {
	var rd *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(creds.BaseURL, "/")+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if creds.Email != "" {
		req.Header.Set("Authorization", "Basic "+
			base64.StdEncoding.EncodeToString([]byte(creds.Email+":"+creds.Token)))
	} else {
		// Data Center / Server personal access tokens use Bearer auth.
		req.Header.Set("Authorization", "Bearer "+creds.Token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := jiraHTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("jira: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return 0, nil, err
	}
	return res.StatusCode, raw, nil
}

func jiraErr(action string, status int, raw []byte) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("jira: %s failed (%d) — check the API token, email, and site permissions", action, status)
	}
	return fmt.Errorf("jira: %s failed (%d): %s", action, status, truncateBytes(raw, 400))
}

// jiraIssueSummary shapes a raw issue into the compact map the tools return.
func jiraIssueSummary(raw json.RawMessage) map[string]any {
	var is struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Status      struct {
				Name string `json:"name"`
			} `json:"status"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Priority struct {
				Name string `json:"name"`
			} `json:"priority"`
			Assignee *struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
			Reporter *struct {
				DisplayName string `json:"displayName"`
			} `json:"reporter"`
			Labels  []string `json:"labels"`
			Created string   `json:"created"`
			Updated string   `json:"updated"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &is); err != nil {
		return nil
	}
	out := map[string]any{
		"key":     is.Key,
		"summary": is.Fields.Summary,
		"status":  is.Fields.Status.Name,
		"type":    is.Fields.IssueType.Name,
		"labels":  is.Fields.Labels,
		"created": is.Fields.Created,
		"updated": is.Fields.Updated,
	}
	if is.Fields.Priority.Name != "" {
		out["priority"] = is.Fields.Priority.Name
	}
	if is.Fields.Assignee != nil {
		out["assignee"] = is.Fields.Assignee.DisplayName
	}
	if is.Fields.Reporter != nil {
		out["reporter"] = is.Fields.Reporter.DisplayName
	}
	// API v2 returns the description as a plain string; be tolerant of ADF
	// objects anyway (v3 / renderer differences) by falling back to raw JSON.
	var desc string
	if json.Unmarshal(is.Fields.Description, &desc) == nil {
		out["description"] = desc
	} else if len(is.Fields.Description) > 0 {
		out["description"] = string(is.Fields.Description)
	}
	return out
}

func validIssueKey(key string) bool {
	if key == "" || len(key) > 64 || !strings.Contains(key, "-") {
		return false
	}
	return !strings.ContainsAny(key, " \t\n/?&")
}

// ── native_jira_get_issue ─────────────────────────────────────────────────────

type JiraGetIssueTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewJiraGetIssueTool(pool *pgxpool.Pool, cfg *config.Config) *JiraGetIssueTool {
	return &JiraGetIssueTool{pool: pool, cfg: cfg}
}

func (t *JiraGetIssueTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_jira_get_issue",
		Description: "Fetch a Jira issue by key: summary, description, status, type, assignee, labels, " +
			"and the most recent comments. Use for full ticket context before planning work.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"key":{"type":"string","description":"Issue key, e.g. PROJ-123"}
		},"required":["key"]}`),
		RiskLevel: "low",
		TimeoutMs: 30000,
	}
}

func (t *JiraGetIssueTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), jiraCreds{BaseURL: t.cfg.JiraBaseURL, Email: t.cfg.JiraEmail, Token: t.cfg.JiraAPIToken}, input)
}

func (t *JiraGetIssueTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, jiraCredsFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *JiraGetIssueTool) run(ctx context.Context, creds jiraCreds, input map[string]any) (any, error) {
	key, _ := input["key"].(string)
	if !validIssueKey(key) {
		return nil, fmt.Errorf("key is required (e.g. PROJ-123)")
	}
	if !creds.ok() {
		return nil, errNoJiraCreds
	}
	status, raw, err := jiraRequest(ctx, creds, http.MethodGet,
		"/rest/api/2/issue/"+url.PathEscape(key)+"?fields=summary,description,status,issuetype,priority,assignee,reporter,labels,created,updated,comment", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("jira: issue %s not found", key)
	}
	if status < 200 || status >= 300 {
		return nil, jiraErr("get issue", status, raw)
	}
	out := jiraIssueSummary(raw)
	if out == nil {
		return nil, fmt.Errorf("jira: unexpected issue response")
	}
	// Append the most recent comments (v2 comment bodies are plain strings).
	var withComments struct {
		Fields struct {
			Comment struct {
				Comments []struct {
					Author struct {
						DisplayName string `json:"displayName"`
					} `json:"author"`
					Body    string `json:"body"`
					Created string `json:"created"`
				} `json:"comments"`
			} `json:"comment"`
		} `json:"fields"`
	}
	if json.Unmarshal(raw, &withComments) == nil {
		all := withComments.Fields.Comment.Comments
		const maxComments = 10
		if len(all) > maxComments {
			all = all[len(all)-maxComments:]
		}
		comments := make([]map[string]any, 0, len(all))
		for _, c := range all {
			comments = append(comments, map[string]any{
				"author": c.Author.DisplayName, "body": c.Body, "created": c.Created,
			})
		}
		out["comments"] = comments
	}
	return out, nil
}

// ── native_jira_search ────────────────────────────────────────────────────────

type JiraSearchTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewJiraSearchTool(pool *pgxpool.Pool, cfg *config.Config) *JiraSearchTool {
	return &JiraSearchTool{pool: pool, cfg: cfg}
}

func (t *JiraSearchTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_jira_search",
		Description: "Search Jira issues with a JQL query (e.g. `project = PROJ AND status = \"In Progress\"`). " +
			"Returns key, summary, status, and assignee per issue.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"jql":{"type":"string","description":"JQL query"},
			"max_results":{"type":"integer","description":"Max issues to return (default 20, max 50)"}
		},"required":["jql"]}`),
		RiskLevel: "low",
		TimeoutMs: 30000,
	}
}

func (t *JiraSearchTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), jiraCreds{BaseURL: t.cfg.JiraBaseURL, Email: t.cfg.JiraEmail, Token: t.cfg.JiraAPIToken}, input)
}

func (t *JiraSearchTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, jiraCredsFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *JiraSearchTool) run(ctx context.Context, creds jiraCreds, input map[string]any) (any, error) {
	jql, _ := input["jql"].(string)
	if strings.TrimSpace(jql) == "" {
		return nil, fmt.Errorf("jql is required")
	}
	if !creds.ok() {
		return nil, errNoJiraCreds
	}
	maxResults := 20
	if v, ok := input["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}
	if maxResults > 50 {
		maxResults = 50
	}
	body := map[string]any{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     []string{"summary", "status", "issuetype", "priority", "assignee", "labels", "created", "updated"},
	}
	// Jira Cloud replaced POST /rest/api/2/search with /search/jql (the old
	// path now 404/410s there); Data Center only has the old one. Try the new
	// path first and fall back.
	status, raw, err := jiraRequest(ctx, creds, http.MethodPost, "/rest/api/2/search/jql", body)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusGone || status == http.StatusMethodNotAllowed {
		status, raw, err = jiraRequest(ctx, creds, http.MethodPost, "/rest/api/2/search", body)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		return nil, jiraErr("search", status, raw)
	}
	var res struct {
		Issues []json.RawMessage `json:"issues"`
		Total  *int              `json:"total"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("jira: unexpected search response")
	}
	issues := make([]map[string]any, 0, len(res.Issues))
	for _, ri := range res.Issues {
		if m := jiraIssueSummary(ri); m != nil {
			delete(m, "description")
			issues = append(issues, m)
		}
	}
	out := map[string]any{"issues": issues, "count": len(issues)}
	if res.Total != nil {
		out["total"] = *res.Total
	}
	return out, nil
}

// ── native_jira_add_comment ───────────────────────────────────────────────────

type JiraAddCommentTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewJiraAddCommentTool(pool *pgxpool.Pool, cfg *config.Config) *JiraAddCommentTool {
	return &JiraAddCommentTool{pool: pool, cfg: cfg}
}

func (t *JiraAddCommentTool) Definition() domain.Tool {
	return domain.Tool{
		Name:        "native_jira_add_comment",
		Description: "Add a comment to a Jira issue. Body is plain text / Jira wiki markup.",
		Type:        "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"key":{"type":"string","description":"Issue key, e.g. PROJ-123"},
			"body":{"type":"string","description":"Comment text"}
		},"required":["key","body"]}`),
		RiskLevel: "medium",
		TimeoutMs: 30000,
	}
}

func (t *JiraAddCommentTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), jiraCreds{BaseURL: t.cfg.JiraBaseURL, Email: t.cfg.JiraEmail, Token: t.cfg.JiraAPIToken}, input)
}

func (t *JiraAddCommentTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, jiraCredsFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *JiraAddCommentTool) run(ctx context.Context, creds jiraCreds, input map[string]any) (any, error) {
	key, _ := input["key"].(string)
	body, _ := input["body"].(string)
	if !validIssueKey(key) || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("key and body are required")
	}
	if !creds.ok() {
		return nil, errNoJiraCreds
	}
	status, raw, err := jiraRequest(ctx, creds, http.MethodPost,
		"/rest/api/2/issue/"+url.PathEscape(key)+"/comment", map[string]any{"body": body})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, jiraErr("add comment", status, raw)
	}
	var c struct {
		ID      string `json:"id"`
		Created string `json:"created"`
	}
	_ = json.Unmarshal(raw, &c)
	return map[string]any{"key": key, "comment_id": c.ID, "created": c.Created}, nil
}

// ── native_jira_transition_issue ──────────────────────────────────────────────

type JiraTransitionIssueTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewJiraTransitionIssueTool(pool *pgxpool.Pool, cfg *config.Config) *JiraTransitionIssueTool {
	return &JiraTransitionIssueTool{pool: pool, cfg: cfg}
}

func (t *JiraTransitionIssueTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_jira_transition_issue",
		Description: "Move a Jira issue to another status (e.g. \"In Review\", \"Done\"). " +
			"Matches the transition or target status name case-insensitively; on no match, returns the available transitions.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"key":{"type":"string","description":"Issue key, e.g. PROJ-123"},
			"status":{"type":"string","description":"Target status or transition name"}
		},"required":["key","status"]}`),
		RiskLevel: "medium",
		TimeoutMs: 30000,
	}
}

func (t *JiraTransitionIssueTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), jiraCreds{BaseURL: t.cfg.JiraBaseURL, Email: t.cfg.JiraEmail, Token: t.cfg.JiraAPIToken}, input)
}

func (t *JiraTransitionIssueTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, jiraCredsFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *JiraTransitionIssueTool) run(ctx context.Context, creds jiraCreds, input map[string]any) (any, error) {
	key, _ := input["key"].(string)
	target, _ := input["status"].(string)
	if !validIssueKey(key) || strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("key and status are required")
	}
	if !creds.ok() {
		return nil, errNoJiraCreds
	}
	transitionsPath := "/rest/api/2/issue/" + url.PathEscape(key) + "/transitions"
	status, raw, err := jiraRequest(ctx, creds, http.MethodGet, transitionsPath, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, jiraErr("list transitions", status, raw)
	}
	var tr struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("jira: unexpected transitions response")
	}
	want := strings.ToLower(strings.TrimSpace(target))
	var id, name string
	available := make([]string, 0, len(tr.Transitions))
	for _, t := range tr.Transitions {
		available = append(available, t.Name+" → "+t.To.Name)
		if strings.ToLower(t.Name) == want || strings.ToLower(t.To.Name) == want {
			id, name = t.ID, t.To.Name
		}
	}
	if id == "" {
		return nil, fmt.Errorf("jira: no transition to %q on %s — available: %s",
			target, key, strings.Join(available, "; "))
	}
	status, raw, err = jiraRequest(ctx, creds, http.MethodPost, transitionsPath,
		map[string]any{"transition": map[string]string{"id": id}})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, jiraErr("transition", status, raw)
	}
	return map[string]any{"key": key, "status": name}, nil
}
