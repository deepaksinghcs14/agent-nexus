#!/usr/bin/env node
/*
 * Capture product screenshots into docs/screenshots/ with the current UI.
 *
 * Auth stays with you: the script logs in via the API using env vars — it never
 * prints or persists your credentials. It reuses the returned access token by
 * seeding it into localStorage before each page loads.
 *
 * Code repositories are treated as sensitive: the Connectors page and the
 * Claude Code *Repositories* tab (which list real repo names) are deliberately
 * skipped. Claude Code is captured on its Sessions tab instead.
 *
 * One-time setup (playwright is intentionally NOT a committed dependency — its
 * postinstall downloads browser binaries, which would break the Docker build):
 *   cd apps/web && npm i -D playwright && npx playwright install chromium
 *
 * Run (from apps/web, with the stack up — `make docker-up`):
 *   NEXUS_EMAIL=you@example.com NEXUS_PASSWORD=•••• node scripts/capture-screenshots.mjs
 *   # or, instead of email+password:  NEXUS_TOKEN=<jwt> node scripts/capture-screenshots.mjs
 *
 * Optional env: WEB_URL (default http://localhost:3000), API_URL (default http://localhost:8080)
 */
import { chromium } from 'playwright'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { mkdirSync } from 'node:fs'

const WEB = process.env.WEB_URL || 'http://localhost:3000'
const API = process.env.API_URL || 'http://localhost:8080'
const EMAIL = process.env.NEXUS_EMAIL
const PASSWORD = process.env.NEXUS_PASSWORD
const TOKEN = process.env.NEXUS_TOKEN // alternative to email+password

if (!TOKEN && !(EMAIL && PASSWORD)) {
  console.error('Provide auth: either NEXUS_TOKEN, or NEXUS_EMAIL + NEXUS_PASSWORD.')
  process.exit(1)
}

// script lives at apps/web/scripts → repo root is three levels up
const OUT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'docs', 'screenshots')
mkdirSync(OUT, { recursive: true })

const VIEWPORT = { width: 1440, height: 900 }
const SCALE = 2 // retina-crisp PNGs

// name → route, plus optional pre-shot actions. Code-repo pages are omitted.
const SHOTS = [
  // ── Core product ───────────────────────────────────────────────
  { file: '02_dashboard', url: '/dashboard' },
  { file: '03_agents_list', url: '/agents' },
  { file: '20_nexus_ai', url: '/nexus-ai' },
  { file: '08_playground', url: '/playground' },
  { file: '09_runs', url: '/runs' },
  { file: '10_tools', url: '/tools' },
  { file: '11_mcp_servers', url: '/mcp-servers' },
  { file: 'skills', url: '/skills' },
  { file: 'gateway', url: '/gateway' },
  { file: '13_workflows', url: '/workflows' },
  { file: '17_memory', url: '/memory' },
  { file: '18_usage', url: '/usage' },
  { file: 'latency', url: '/observability' },
  { file: 'triggers_01_list', url: '/triggers' },
  { file: 'triggers_02_new_form', url: '/triggers/new' },
  { file: 'triggers_03_new_prefilled', url: '/triggers/new' },

  // ── Agent builder tabs ─────────────────────────────────────────
  { file: '04_agent_builder_basics', url: '/agents/new' },
  { file: '05_agent_builder_model', url: '/agents/new', tab: 'Model' },
  { file: '06_agent_builder_tools', url: '/agents/new', tab: 'Tools' },
  { file: '07_agent_builder_memory', url: '/agents/new', tab: 'Memory' },

  // ── Claude Code — Sessions tab only (Repositories tab lists repos) ─
  { file: 'claude_code', url: '/claude-code', tab: 'Sessions' },

  // ── Settings ───────────────────────────────────────────────────
  { file: '19_providers', url: '/settings/providers' },
  { file: 'api_tokens', url: '/settings/api-tokens' },
  { file: 'workspace', url: '/settings/workspace' },

  // ── Admin (emails allowed) ─────────────────────────────────────
  { file: 'admin_01_overview', url: '/admin/overview' },
  { file: 'admin_02_users', url: '/admin/users' },
  { file: 'admin_03_workspaces', url: '/admin/workspaces' },
  { file: 'admin_04_audit_logs', url: '/admin/audit-logs' },
  { file: 'admin_05_policies', url: '/admin/policies' },

  // ── Docs (restyled shell + prose) ──────────────────────────────
  { file: 'docs_01_what_is_an_agent', url: '/docs/what-is-an-agent' },
  { file: 'docs_02_agent_configuration', url: '/docs/agent-configuration' },
  { file: 'docs_03_invoke_api', url: '/docs/invoke-api' },
  { file: 'docs_05_mcp_servers', url: '/docs/mcp-servers' },
  { file: 'docs_06_sse_events', url: '/docs/sse-events' },
  { file: 'docs_07_workflows', url: '/docs/workflows' },
]

async function login() {
  const res = await fetch(`${API}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: EMAIL, password: PASSWORD }),
  })
  if (!res.ok) throw new Error(`login failed (${res.status})`)
  const { access_token } = await res.json()
  if (!access_token) throw new Error('no access_token in login response')
  return access_token
}

// Pull the first N workflows so we can screenshot their canvases. Workflow
// names are shown; if any of yours embed a private repo name, drop them here.
async function workflowShots(token, n = 2) {
  try {
    const res = await fetch(`${API}/api/v1/workflows`, { headers: { Authorization: `Bearer ${token}` } })
    if (!res.ok) return []
    const { data } = await res.json()
    return (data || []).slice(0, n).map((wf, i) => ({
      file: i === 0 ? '14_workflow_canvas' : `15_workflow_canvas_${i + 1}`,
      url: `/workflows/${wf.id}`,
      canvas: true,
    }))
  } catch { return [] }
}

async function main() {
  const token = TOKEN || (console.log('Logging in…'), await login())

  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: VIEWPORT, deviceScaleFactor: SCALE })
  // Seed the token into localStorage before any app script runs…
  await context.addInitScript((t) => { try { localStorage.setItem('access_token', t) } catch {} }, token)
  // …and the has_session cookie the Next middleware gates every route on
  // (otherwise heavier routes redirect to /login before the client sets it).
  await context.addCookies([{ name: 'has_session', value: '1', url: WEB }])
  const page = await context.newPage()

  const allShots = [...SHOTS, ...(await workflowShots(token))]
  const only = (process.env.ONLY || '').split(',').map((s) => s.trim()).filter(Boolean)
  const shots = only.length ? allShots.filter((s) => only.includes(s.file)) : allShots

  let ok = 0
  for (const shot of shots) {
    try {
      await page.goto(WEB + shot.url, { waitUntil: 'networkidle', timeout: 30000 })
      await page.waitForTimeout(shot.canvas ? 2500 : 900) // React Flow needs longer to lay out
      if (shot.tab) {
        // Click the actual tab control. Match on the accessible NAME anchored at
        // the start (so "Tools 1"/"Sessions (8)" still match) via role=tab
        // (Radix) or, failing that, a role=button in <main> (custom tab bars).
        // Anchoring + role avoids matching the tab-bar container or same-named
        // sidebar nav items.
        const rx = new RegExp('^\\s*' + shot.tab.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
        let target = page.getByRole('tab', { name: rx })
        if (!(await target.count())) target = page.locator('main').getByRole('button', { name: rx })
        target = target.first()
        if (await target.count()) { await target.click().catch(() => {}); await page.waitForTimeout(800) }
      }
      const dest = resolve(OUT, `${shot.file}.png`)
      await page.screenshot({ path: dest })
      console.log(`✓ ${shot.file}.png  (${shot.url}${shot.tab ? ' · ' + shot.tab : ''})`)
      ok++
    } catch (e) {
      console.warn(`✗ ${shot.file}  — ${e.message}`)
    }
  }

  await browser.close()
  console.log(`\nDone: ${ok}/${shots.length} captured into docs/screenshots/`)
  console.log('Skipped by design (code repos): Connectors, Claude Code Repositories tab, Conversations.')
}

main().catch((e) => { console.error(e); process.exit(1) })
