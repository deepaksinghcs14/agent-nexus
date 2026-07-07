package tools

import "strings"

// Functional tool/skill categories. Curated set — keep in sync with the web
// taxonomy in apps/web/src/lib/tool-category.ts.
const (
	CatCommunication = "Communication"
	CatWebSearch     = "Web & Search"
	CatDevCode       = "Dev & Code"
	CatDataHTTP      = "Data & HTTP"
	CatMemory        = "Memory & Context"
	CatOrchestration = "Orchestration"
	CatKnowledge     = "Knowledge"
	CatAI            = "AI"
	CatGeneral       = "General"
)

// Categories is the canonical ordered list of functional categories, exposed via
// the API so the UI can render a consistent dropdown/filter set.
var Categories = []string{
	CatCommunication, CatWebSearch, CatDevCode, CatDataHTTP,
	CatMemory, CatOrchestration, CatKnowledge, CatAI, CatGeneral,
}

// CategoryForTool returns the functional category for a tool by name + type.
// Used by Registry.SeedDB to populate the category column for native tools, and
// available for categorizing custom tools consistently.
func CategoryForTool(name, toolType string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "whatsapp_"),
		strings.Contains(n, "send_message"),
		strings.Contains(n, "messaging"),
		strings.Contains(n, "ask_user"):
		return CatCommunication
	case strings.Contains(n, "web_search"), strings.Contains(n, "summarize_link"):
		return CatWebSearch
	case strings.HasPrefix(n, "github_"), strings.HasPrefix(n, "jira_"),
		strings.HasPrefix(n, "code_"), strings.Contains(n, "pull_request"),
		strings.Contains(n, "branch_diff"), strings.Contains(n, "repo_session"),
		strings.Contains(n, "review_session"), strings.Contains(n, "code_tool"),
		toolType == "code":
		return CatDevCode
	case strings.Contains(n, "http_request"), strings.Contains(n, "http_tool"),
		toolType == "http":
		return CatDataHTTP
	case strings.Contains(n, "memory"):
		return CatMemory
	case strings.Contains(n, "retrieve_context"), strings.Contains(n, "connector"):
		// Knowledge/RAG: context retrieval and connector (data-source) management.
		return CatKnowledge
	case strings.Contains(n, "agent"), strings.Contains(n, "workflow"),
		strings.Contains(n, "skill"), strings.Contains(n, "tool"),
		strings.Contains(n, "promote_resource"), strings.Contains(n, "launch_session"):
		// Meta/management tools that build and wire up the platform itself.
		return CatOrchestration
	case strings.Contains(n, "read_file"), strings.Contains(n, "write_file"):
		return CatDataHTTP
	default:
		return CatGeneral
	}
}
