# Latency Dashboard — Implementation Plan

## Goal

Add a **Latency** page under the Observe section (`/observability`) showing P50/P95 run duration broken down by agent, by model/provider, and as a daily trend over the last 7/14/30 days. No new chart library needed — use CSS bar visualisations like the existing Usage page.

---

## 1. Backend — new handler file

**Create `services/api/internal/api/handler/observability.go`**

```go
package handler

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
    "github.com/jackc/pgx/v5/pgxpool"
)

type ObservabilityHandler struct {
    pool *pgxpool.Pool
}

func NewObservabilityHandler(pool *pgxpool.Pool) *ObservabilityHandler {
    return &ObservabilityHandler{pool: pool}
}
```

### `GET /api/v1/observability/latency?days=7`

Single endpoint returning three sections in one JSON response:

```json
{
  "by_agent": [...],
  "by_model": [...],
  "trend": [...]
}
```

#### `by_agent` query

```sql
SELECT
  a.id,
  a.name,
  a.provider,
  a.model,
  COUNT(*)                                                                          AS run_count,
  COUNT(*) FILTER (WHERE r.status = 'success')                                     AS success_count,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3)   AS p50_secs,
  ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3)   AS p95_secs,
  ROUND(AVG(r.total_input_tokens)::numeric, 0)                                     AS avg_input_tokens,
  ROUND(AVG(r.total_output_tokens)::numeric, 0)                                    AS avg_output_tokens
FROM runs r
JOIN agents a ON a.id = r.agent_id
WHERE r.workspace_id = $1::uuid
  AND r.completed_at IS NOT NULL
  AND r.started_at > NOW() - ($2 || ' days')::interval
GROUP BY a.id, a.name, a.provider, a.model
ORDER BY run_count DESC
LIMIT 20
```

Parameters: `$1` = workspace_id, `$2` = days (int, default 7).

#### `by_model` query

```sql
SELECT
  a.provider,
  a.model,
  COUNT(*)                                                                                    AS run_count,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3)             AS p50_secs,
  ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3)             AS p95_secs,
  ROUND(AVG(
      r.total_output_tokens::float /
      NULLIF(EXTRACT(EPOCH FROM (r.completed_at - r.started_at)), 0)
  )::numeric, 1)                                                                              AS tokens_per_sec
FROM runs r
JOIN agents a ON a.id = r.agent_id
WHERE r.workspace_id = $1::uuid
  AND r.completed_at IS NOT NULL
  AND r.status = 'success'
  AND r.started_at > NOW() - ($2 || ' days')::interval
GROUP BY a.provider, a.model
ORDER BY run_count DESC
```

#### `trend` query (daily P50 / P95)

```sql
SELECT
  DATE_TRUNC('day', r.started_at AT TIME ZONE 'UTC')::date::text   AS day,
  COUNT(*)                                                           AS run_count,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3) AS p50_secs,
  ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3) AS p95_secs
FROM runs r
WHERE r.workspace_id = $1::uuid
  AND r.completed_at IS NOT NULL
  AND r.status = 'success'
  AND r.started_at > NOW() - ($2 || ' days')::interval
GROUP BY 1
ORDER BY 1
```

Run all three queries sequentially, scan into typed structs, return as JSON:

```json
{
  "by_agent": [
    { "id": "...", "name": "Deepak", "provider": "gemini", "model": "gemini-2.5-flash",
      "run_count": 42, "success_count": 41, "p50_secs": 1.54, "p95_secs": 4.2,
      "avg_input_tokens": 2850, "avg_output_tokens": 28 }
  ],
  "by_model": [
    { "provider": "gemini", "model": "gemini-2.5-flash",
      "run_count": 55, "p50_secs": 1.48, "p95_secs": 3.9, "tokens_per_sec": 18.4 }
  ],
  "trend": [
    { "day": "2026-06-11", "run_count": 7, "p50_secs": 2.1, "p95_secs": 5.3 }
  ]
}
```

---

## 2. Register handler

**`services/api/internal/api/handler/handlers.go`** — add field:

```go
Observability *ObservabilityHandler
```

**`services/api/internal/api/router/router.go`** — inside the authenticated `r.Group` (same level as `/runs`, `/usage`):

```go
r.Get("/observability/latency", h.Observability.Latency)
```

**`services/api/cmd/server/main.go`** — wire in `NewObservabilityHandler(pool)` alongside the other handlers when constructing `handler.Handlers{}`.

---

## 3. API client

**`apps/web/src/lib/api.ts`** — add:

```ts
export const observabilityAPI = {
  latency: (days = 7) => api.get(`/observability/latency?days=${days}`),
}
```

---

## 4. Types

**`apps/web/src/types/index.ts`** — add:

```ts
export interface LatencyByAgent {
  id: string
  name: string
  provider: string
  model: string
  run_count: number
  success_count: number
  p50_secs: number
  p95_secs: number
  avg_input_tokens: number
  avg_output_tokens: number
}

export interface LatencyByModel {
  provider: string
  model: string
  run_count: number
  p50_secs: number
  p95_secs: number
  tokens_per_sec: number
}

export interface LatencyTrendDay {
  day: string
  run_count: number
  p50_secs: number
  p95_secs: number
}

export interface LatencyData {
  by_agent: LatencyByAgent[]
  by_model: LatencyByModel[]
  trend: LatencyTrendDay[]
}
```

---

## 5. Frontend page

**Create `apps/web/src/app/observability/page.tsx`**

### Structure

```
/observability
  ├── Time-range selector tabs: 7d | 14d | 30d  (state: days)
  ├── Section: "By Agent"
  │     Table with columns:
  │       Agent | Provider | Model | P50 | P95 | Success rate | Runs | Avg tokens in
  │     P50/P95 rendered as  "1.54s" in a small pill (green <2s, amber 2-5s, red >5s)
  │     Success rate as a small % badge (green ≥95%, amber 80-95%, red <80%)
  │
  ├── Section: "By Model"
  │     Table with columns:
  │       Provider | Model | P50 | P95 | Tokens/sec | Runs
  │     Same colour-coded pills for P50/P95
  │
  └── Section: "Daily Trend (P50 / P95)"
        CSS bar chart — no library needed.
        For each day: two stacked/side-by-side bars (P50=blue, P95=purple).
        Bar height = (value / maxP95) * 100%.
        X-axis labels: short date (Jun 11).
        Tooltip on hover showing date, run count, P50, P95.
```

### Colour helper

```ts
function latencyColor(secs: number): string {
  if (secs < 2) return 'bg-green-100 text-green-700'
  if (secs < 5) return 'bg-amber-100 text-amber-700'
  return 'bg-red-100 text-red-700'
}
```

### Bar chart (no recharts)

```tsx
// maxVal = Math.max(...trend.map(d => d.p95_secs), 1)
// For each day d:
<div className="flex flex-col items-center gap-0.5" key={d.day}>
  <div className="flex items-end gap-0.5 h-24">
    <div
      title={`P50: ${d.p50_secs}s`}
      className="w-3 bg-blue-400 rounded-t"
      style={{ height: `${(d.p50_secs / maxVal) * 100}%` }}
    />
    <div
      title={`P95: ${d.p95_secs}s`}
      className="w-3 bg-purple-400 rounded-t opacity-70"
      style={{ height: `${(d.p95_secs / maxVal) * 100}%` }}
    />
  </div>
  <span className="text-[10px] text-gray-400 rotate-45 origin-left mt-2">
    {d.day.slice(5)} {/* MM-DD */}
  </span>
</div>
```

### Empty state

When `by_agent` is empty: centered icon + "No completed runs in this period."

---

## 6. Sidebar nav

**`apps/web/src/components/layout/Sidebar.tsx`**

Add `Timer` to the lucide-react import line, then insert into the `Observe` group after `Usage`:

```ts
{ label: 'Latency', href: '/observability', icon: Timer },
```

---

## Checklist

- [ ] `services/api/internal/api/handler/observability.go` — handler with Latency method
- [ ] `services/api/internal/api/handler/handlers.go` — add `Observability *ObservabilityHandler`
- [ ] `services/api/cmd/server/main.go` — wire `NewObservabilityHandler(pool)`
- [ ] `services/api/internal/api/router/router.go` — register `GET /observability/latency`
- [ ] `apps/web/src/lib/api.ts` — add `observabilityAPI`
- [ ] `apps/web/src/types/index.ts` — add 4 new types
- [ ] `apps/web/src/app/observability/page.tsx` — new page
- [ ] `apps/web/src/components/layout/Sidebar.tsx` — add Latency nav item
