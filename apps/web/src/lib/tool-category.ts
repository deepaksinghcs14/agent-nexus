import type { Tool } from '@/types'

// Canonical functional categories — keep in sync with the Go taxonomy in
// services/api/internal/tools/category.go.
export const CATEGORIES = [
  'Communication',
  'Web & Search',
  'Dev & Code',
  'Data & HTTP',
  'Memory & Context',
  'Orchestration',
  'Knowledge',
  'AI',
  'General',
] as const

// toolCategory returns a tool's display category. It prefers the explicit
// `category` field (set in the backend) and falls back to a heuristic derived
// from the tool's type/name so tools seeded before categories existed still group
// sensibly.
export function toolCategory(tool: Tool): string {
  if (tool.category && tool.category.trim()) return tool.category
  if (tool.name.startsWith('whatsapp_')) return 'Communication'
  if (tool.name.startsWith('mcp_') || tool.type === 'mcp') return 'MCP'
  if (tool.type === 'http') return 'HTTP'
  if (tool.type === 'code' || tool.name.startsWith('code_')) return 'Code'
  return 'Built-in'
}
