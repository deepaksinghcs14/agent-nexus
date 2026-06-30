package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunClaudeCodeTool struct{ pool *pgxpool.Pool }

func NewRunClaudeCodeTool(pool *pgxpool.Pool) *RunClaudeCodeTool {
	return &RunClaudeCodeTool{pool: pool}
}

func (t *RunClaudeCodeTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "Full description of the code change to implement. Be specific — include files, functions, and expected behaviour.",
			},
			"github_connector_id": map[string]any{
				"type":        "string",
				"description": "UUID of a connected GitHub connector. When provided, repo_url and github_token are looked up automatically from the connector config. Preferred over passing raw credentials.",
			},
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL (e.g. https://github.com/org/repo). Required if github_connector_id is not provided.",
			},
			"github_token": map[string]any{
				"type":        "string",
				"description": "GitHub personal access token with repo write permission. Required if github_connector_id is not provided.",
			},
			"branch_name": map[string]any{
				"type":        "string",
				"description": "Branch to create or update. Defaults to 'ai/nexus-{timestamp}' if omitted.",
			},
			"base_branch": map[string]any{
				"type":        "string",
				"description": "Branch to clone from. Defaults to 'main'.",
			},
			"pull_existing": map[string]any{
				"type":        "boolean",
				"description": "If true, checks out the existing branch and pulls instead of creating a new one. Use for review-cycle iterations where the PR already exists.",
			},
			"create_pr": map[string]any{
				"type":        "boolean",
				"description": "If false, skips PR creation after pushing (the branch is pushed but no PR is opened). Defaults to true.",
			},
			"pr_title": map[string]any{
				"type":        "string",
				"description": "Pull request title. Defaults to the first line of the task.",
			},
			"pr_body": map[string]any{
				"type":        "string",
				"description": "Pull request description. Auto-generated from the task if omitted.",
			},
		},
		"required": []string{"task"},
	})
	return domain.Tool{
		Name:             "native_run_claude_code",
		Description:      "Run Claude Code CLI on a GitHub repository to implement a coding task end-to-end: clones the repo, lets Claude Code read/edit/test files, commits all changes, pushes the branch, and creates a GitHub pull request. Uses the server's 'claude login' OAuth session or ANTHROPIC_API_KEY env var. Prefer github_connector_id over raw credentials.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "high",
		RequiresApproval: false,
		TimeoutMs:        600000,
		Enabled:          true,
	}
}

func (t *RunClaudeCodeTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_run_claude_code requires run context")
}

func (t *RunClaudeCodeTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	task, _ := input["task"].(string)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	// Resolve GitHub credentials.
	repoURL, token, err := t.resolveGitHubCreds(ctx, execCtx.WorkspaceID, input)
	if err != nil {
		return nil, err
	}

	// Parse owner/repo from URL.
	owner, repo, err := parseGitHubRepo(repoURL)
	if err != nil {
		return nil, err
	}

	// Branch setup.
	baseBranch, _ := input["base_branch"].(string)
	if baseBranch == "" {
		baseBranch = "main"
	}
	branchName, _ := input["branch_name"].(string)
	if branchName == "" {
		branchName = fmt.Sprintf("ai/nexus-%d", time.Now().Unix())
	}
	pullExisting, _ := input["pull_existing"].(bool)
	createPR := true
	if v, ok := input["create_pr"].(bool); ok {
		createPR = v
	}

	prTitle, _ := input["pr_title"].(string)
	if prTitle == "" {
		prTitle = firstLine(task)
	}
	prBody, _ := input["pr_body"].(string)
	if prBody == "" {
		prBody = fmt.Sprintf("Automated code change implemented by Agent Nexus.\n\n**Task:**\n%s", task)
	}

	// Clone repo to a temp directory.
	workDir, err := os.MkdirTemp("", "nexus-cc-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	authURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	if err := runGit(ctx, "", "clone", "--depth=1", "--branch="+baseBranch, authURL, workDir); err != nil {
		// Try default branch if baseBranch doesn't exist.
		if err2 := runGit(ctx, "", "clone", "--depth=1", authURL, workDir); err2 != nil {
			return nil, fmt.Errorf("git clone: %w", err)
		}
	}

	// Configure git identity in the cloned repo.
	runGit(ctx, workDir, "config", "user.name", "Agent Nexus")    //nolint:errcheck
	runGit(ctx, workDir, "config", "user.email", "agent@nexus.ai") //nolint:errcheck

	// Branch checkout.
	if pullExisting {
		if err := runGit(ctx, workDir, "fetch", "origin", branchName); err != nil {
			return nil, fmt.Errorf("git fetch branch %s: %w", branchName, err)
		}
		if err := runGit(ctx, workDir, "checkout", branchName); err != nil {
			return nil, fmt.Errorf("git checkout %s: %w", branchName, err)
		}
		runGit(ctx, workDir, "pull", "origin", branchName) //nolint:errcheck
	} else {
		if err := runGit(ctx, workDir, "checkout", "-b", branchName); err != nil {
			return nil, fmt.Errorf("git checkout -b %s: %w", branchName, err)
		}
	}

	// Build the prompt sent to Claude Code.
	prompt := buildClaudePrompt(task)

	// Run claude --print.
	claudeOutput, err := runClaudeCode(ctx, workDir, prompt)
	if err != nil {
		return nil, fmt.Errorf("claude code: %w", err)
	}

	// Push branch.
	pushArgs := []string{"push", "origin", branchName}
	if pullExisting {
		pushArgs = append(pushArgs, "--force-with-lease")
	}
	if err := runGit(ctx, workDir, pushArgs...); err != nil {
		return nil, fmt.Errorf("git push: %w", err)
	}

	result := map[string]any{
		"output": claudeOutput,
		"branch": branchName,
		"repo":   fmt.Sprintf("%s/%s", owner, repo),
	}

	if !createPR {
		result["note"] = "Branch pushed. PR creation skipped (create_pr=false). New commits will appear in the existing open PR."
		return result, nil
	}

	// Create GitHub PR.
	prURL, err := createGitHubPR(ctx, token, owner, repo, branchName, baseBranch, prTitle, prBody)
	if err != nil {
		result["pr_error"] = err.Error()
		result["note"] = "Branch pushed successfully but PR creation failed. You can create the PR manually."
		return result, nil
	}
	result["pr_url"] = prURL
	return result, nil
}

// resolveGitHubCreds returns (repoURL, token, error).
// If github_connector_id is given, looks up connector config from DB.
// Otherwise uses repo_url + github_token from input.
func (t *RunClaudeCodeTool) resolveGitHubCreds(ctx context.Context, workspaceID string, input map[string]any) (string, string, error) {
	if connID, ok := input["github_connector_id"].(string); ok && connID != "" {
		var configJSON []byte
		if err := t.pool.QueryRow(ctx,
			`SELECT config FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid`,
			connID, workspaceID,
		).Scan(&configJSON); err != nil {
			return "", "", fmt.Errorf("github_connector_id %s not found in workspace: %w", connID, err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return "", "", fmt.Errorf("parse connector config: %w", err)
		}
		repoURL, _ := cfg["repo_url"].(string)
		tok, _ := cfg["token"].(string)
		owner, _ := cfg["owner"].(string)
		repo, _ := cfg["repo"].(string)

		if repoURL == "" && owner != "" && repo != "" {
			repoURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
		}
		if repoURL == "" {
			return "", "", fmt.Errorf("connector config missing repo_url (owner=%q repo=%q)", owner, repo)
		}
		if tok == "" {
			return "", "", fmt.Errorf("connector config missing token")
		}
		return repoURL, tok, nil
	}

	repoURL, _ := input["repo_url"].(string)
	token, _ := input["github_token"].(string)
	if repoURL == "" {
		return "", "", fmt.Errorf("either github_connector_id or repo_url is required")
	}
	if token == "" {
		return "", "", fmt.Errorf("github_token is required when github_connector_id is not provided")
	}
	return repoURL, token, nil
}

func parseGitHubRepo(repoURL string) (owner, repo string, err error) {
	u := strings.TrimSuffix(repoURL, ".git")
	u = strings.TrimPrefix(u, "https://github.com/")
	u = strings.TrimPrefix(u, "http://github.com/")
	u = strings.TrimPrefix(u, "github.com/")
	parts := strings.SplitN(u, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse owner/repo from URL %q", repoURL)
	}
	return parts[0], parts[1], nil
}

func buildClaudePrompt(task string) string {
	return fmt.Sprintf(`You are implementing a code change in a git repository already cloned to your working directory.

TASK:
%s

RULES:
- Make all code changes to implement the task completely.
- git add and commit ALL modified files with a clear, descriptive commit message.
- Do NOT run 'git push' — pushing is handled automatically after you finish.
- Do NOT create a GitHub PR — that is handled automatically after you finish.
- Run existing tests if available; fix any failures before committing.
- Make reasonable assumptions when the task is ambiguous; document them in the commit message.`, task)
}

func runClaudeCode(ctx context.Context, workDir, prompt string) (string, error) {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("'claude' CLI not found in PATH — install Claude Code and run 'claude login' on the server")
	}

	cmd := exec.CommandContext(ctx, claudeBin, "--print", "--dangerously-skip-permissions", "-p", prompt)
	cmd.Dir = workDir

	// Inherit server env so stored OAuth session is picked up automatically.
	env := os.Environ()
	// Append git identity overrides.
	env = append(env,
		"GIT_AUTHOR_NAME=Agent Nexus",
		"GIT_AUTHOR_EMAIL=agent@nexus.ai",
		"GIT_COMMITTER_NAME=Agent Nexus",
		"GIT_COMMITTER_EMAIL=agent@nexus.ai",
	)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := stderr.String()
		if errStr == "" {
			errStr = err.Error()
		}
		return "", fmt.Errorf("claude exited with error: %s", errStr)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		out = "(Claude Code produced no output)"
	}
	return out, nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func createGitHubPR(ctx context.Context, token, owner, repo, head, base, title, body string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	})

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 422 {
		// PR may already exist — extract the existing PR URL from the error.
		var errResp struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			for _, e := range errResp.Errors {
				if strings.Contains(e.Message, "pull request already exists") {
					// Fall through — caller's note will handle this.
					return "", fmt.Errorf("pull request already exists for branch %s", head)
				}
			}
		}
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var pr struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &pr); err != nil {
		return "", fmt.Errorf("parse github response: %w", err)
	}
	return pr.HTMLURL, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 72 {
		s = s[:72] + "…"
	}
	return s
}
