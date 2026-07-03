-- OAuth 2.1 support for remote MCP servers (e.g. Atlassian's hosted server).
-- auth_type 'config' keeps the existing static-token behavior; 'oauth' stores
-- an AES-256-GCM-encrypted JSON document (client registration, tokens,
-- pending PKCE flow state) in the oauth column.
ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS auth_type TEXT NOT NULL DEFAULT 'config';
ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS oauth TEXT;
