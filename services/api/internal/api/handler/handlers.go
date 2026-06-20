package handler

// Handlers is the top-level container wired in main.go.
// Each sub-handler is implemented in its own file.
type Handlers struct {
	Auth            *AuthHandler
	Providers       *ProvidersHandler
	Agents          *AgentsHandler
	Tools           *ToolsHandler
	MCP             *MCPHandler
	Connectors      *ConnectorsHandler
	Conversations   *ConversationsHandler
	Workspace       *WorkspaceHandler
	Runs            *RunsHandler
	Memory          *MemoryHandler
	Workflows       *WorkflowsHandler
	Admin           *AdminHandler
	APITokens       *APITokensHandler
	Invoke          *InvokeHandler
	WebhookTriggers *WebhookTriggerHandler
	WebhookIngress  *WebhookIngressHandler
	Gateway         *GatewayHandler
	Skills          *SkillsHandler
	Config          *ConfigHandler
	NexusAI         *NexusAIHandler
	Observability   *ObservabilityHandler
	WhatsAppCreds   *WhatsAppCredsHandler
}
