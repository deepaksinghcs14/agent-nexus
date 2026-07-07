# Fast-Track Agent Creation via Nexus AI

**Goal:** Collapse the ~30-decision manual agent form into a single natural-language
sentence. A user describes what they want, **Nexus AI drafts a complete, runnable
agent** (instructions + model + tools + skills + context + memory), the user
**reviews it in the pre-filled form**, and one click creates it. After creation the
agent can attach/detach its own tools, skills, context, and memory.

## What already exists (do NOT rebuild)

- **Nexus AI chat engine** — `services/api/internal/api/handler/nexus_ai.go`, route
  `POST /api/v1/nexus-ai/chat` (SSE). Own tool-call loop, own tool set.
- **`create_agent` Nexus tool** — already accepts the full config (provider, model,
  temperature, max_tokens, max_steps/tool_calls/duration, memory_enabled/scope,
  context_retrieval_enabled, agentic_rag, tool_ids/tool_names, connector_ids,
  max_chunks, min_score, status). This is the atomic provisioner.
- **Full-catalog visibility** — `list_tools` (whole workspace), `list_connectors`,
  `list_skills`, `list_agents`, `list_available_models`, `list_workflows`.
- **Management tools** — `attach_tool_to_agent`, `detach_tool_from_agent`,
  `attach_skills_to_agent`, `update_agent`, `create_skill`, `create_code_tool`.
- **Frontend** — `apps/web/src/app/nexus-ai/page.tsx` (chat UI, provider/model
  selector, template cards); `nexusAIAPI.chat` in `apps/web/src/lib/api.ts`.
- **Agent runtime self-management** — native `attach_tool`/`detach_tool`,
  `attach_skill`/`detach_skill`, `update_agent`; Agent Self-Management skill; lazy
  tool loading (`invoke.go`).
- **Manual create form** — `apps/web/src/app/agents/new/page.tsx` (854 lines, 8 tabs:
  Basics / Model / Instructions / Skills / Tools / Context / Memory / Guardrails).

## The four gaps to close

1. **Draft, not silent commit.** Nexus `create_agent` persists immediately and drops
   the user on `/agents`. We want it to produce a **draft the user reviews in the
   form** before saving.
2. **Nexus isn't in the create flow.** It lives on a separate `/nexus-ai` page. The
   fast track must live on `/agents/new`.
3. **The manual form explains nothing.** 30 fields, zero inline help ("understand it
   yourself"). Every field needs a plain-language explanation.
4. **Post-creation self-management is partial.** No connector (context) attach/detach;
   `update_agent` can't change memory/connectors; created agents don't get lazy
   loading + Self-Management by default.

---

## Phase 1 — Draft mode + fill management gaps (backend)

**1a. Draft proposal path.** Add a Nexus tool `propose_agent` with the *same schema*
as `create_agent` but which **does not write to the DB**. It validates + resolves
tool/connector/skill names to real records, then emits an SSE event
`{"type":"agent_draft","draft":{…}}` carrying the full resolved spec plus a short
`rationale` per non-obvious choice. Add a builder-mode system-prompt variant that
tells Nexus to call `propose_agent` (not `create_agent`) when the intent is "create
an agent." Files: `nexus_ai.go` (tool def + `toolProposeAgent`, `executeTool`
dispatch, SSE event), keep `create_agent` untouched for backward compat.

**1b. Fill `update_agent` gaps.** Extend the Nexus `update_agent` schema + handler to
accept `temperature`, `max_tokens`, `memory_enabled`, `memory_scope`, `connector_ids`
(with `context_retrieval_enabled`). Files: `nexus_ai.go` (`toolUpdateAgent`).

**1c. Connector attach/detach.** Add native tools `native_attach_connector` /
`native_detach_connector` so a running agent can manage its own context, plus Nexus
`attach_connector_to_agent` / `detach_connector_from_agent`. Files:
`services/api/internal/tools/native/` (new file `connectors_mgmt.go`), register in the
native registry; `nexus_ai.go` for the Nexus side.

## Phase 2 — "Create with Nexus" in the create UI (frontend)

Add a **"Describe your agent"** panel at the top of `/agents/new`. Reuse the SSE chat
client from `nexus-ai/page.tsx` via `nexusAIAPI.chat`. On the `agent_draft` event,
**pre-fill every form field** (name, description, instructions, provider/model, tools,
skills, connectors, memory config, guardrails) and show a "Review & Create" banner
with an inline **rationale panel** ("why these tools / this model"). The user edits
anything and clicks the existing Create button → same endpoint, same validation.
Files: `apps/web/src/app/agents/new/page.tsx`, small extract of the SSE reader into a
shared hook (`apps/web/src/lib/useNexusChat.ts`) reused by both pages.

## Phase 3 — Explain every field (frontend)

Add an `InfoTip` component (info icon → popover) next to each field and a one-line
helper under each tab header, sourced from the existing `/docs/*` pages. Cover:
provider/model/temperature/max_tokens; memory scope/save mode/review policy/min
importance/dedupe/max memories/min relevance; context RAG/agentic RAG/max chunks/min
score; guardrails max steps/tool calls/duration/history; lazy tool loading; compaction
thresholds. Files: new `apps/web/src/components/ui/InfoTip.tsx`; wire into
`agents/new/page.tsx` and `agents/[agentId]/edit/page.tsx`. Copy lives in one map
(`apps/web/src/lib/field-help.ts`) so both forms stay in sync.

## Phase 4 — Born self-managing + Nexus full-resource decisioning

- **Default self-management.** Nexus `create_agent`/`propose_agent` set
  `lazy_tool_loading=true` and attach the **Agent Self-Management skill** by default,
  so a new agent can attach/detach its own tools/skills/context/memory afterward.
  Files: `nexus_ai.go` (`toolCreateAgent`).
- **Memory visibility for Nexus.** Add `list_memories` to the Nexus tool set so it can
  factor existing memory into decisions. Files: `nexus_ai.go`.
- **(Follow-up, not today) Catalog retrieval at scale.** Embed tool/skill
  name+description and make `list_tools` a top-k vector search so picks stay
  token-bounded past a few hundred tools. Reuses `execCtx.Embed` + pgvector. Tracked
  separately.

## Build order (today)

1. Phase 1a `propose_agent` + `agent_draft` SSE — unblocks the whole flow.
2. Phase 1b/1c update_agent + connector tools.
3. Phase 2 draft-into-form UI.
4. Phase 3 field explanations.
5. Phase 4 defaults + memory visibility.

## Non-goals / risks

- `create_agent` stays as-is (backward compat with existing orchestration flows).
- No change to the create-agent REST endpoint or its validation.
- Catalog embedding/retrieval (Phase 4 follow-up) is deferred — flagged because
  `SeedDB` overwrites native tool rows on every startup, so any embedding column must
  survive that unconditional UPDATE.
