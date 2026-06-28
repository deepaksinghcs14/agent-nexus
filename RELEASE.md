# Release Notes

## 2026-06-28

### Agentic RAG

Agents can now retrieve from their connected knowledge sources on-demand during a run instead of getting pre-injected context at the start.

**How it works:** Enable the **Agentic RAG** toggle in the agent's Context tab. When enabled, pre-run retrieval is skipped and the agent gets a `native_retrieve_context(query, max_chunks, min_score)` tool it can call at any point during execution — letting it decide what to look up and when.

**Fixes in this release:**
- `native_retrieve_context` was invisible to agents with Lazy Tool Loading enabled (pre-seeded in `requestedTools` now)
- Gemini agents were hallucinating a `search` tool — system prompt now explicitly directs them to `native_retrieve_context`
- Retrieval now falls back to full-text + keyword search (PostgreSQL `tsvector`) when chunk embeddings are missing, so results are returned even before a connector re-sync

**Setup:** Requires an embedding model. Configure `EMBED_OLLAMA_URL` pointing to an Ollama instance and run `ollama pull nomic-embed-text`. After pulling the model, re-sync your connectors from the Connectors page to populate embeddings.

---

### Mobile-Responsive UI + PWA

The entire app is now fully responsive on mobile.

- **Navigation:** Hamburger button opens a slide-in sidebar drawer on small screens
- **Add to Home Screen:** A banner prompts mobile users to install the app as a PWA
  - Android: native Chrome install prompt
  - iOS: step-by-step Safari share sheet guide
- All pages updated with responsive layouts: agents, workflows, tools, MCP servers, connectors, gateway, triggers, skills, runs, playground, conversations, memory, usage, observability, Nexus AI, settings, admin portal (6 pages), and docs

**Data tables** scroll horizontally on mobile. **Grids** collapse to single column. **Modals** fit within the viewport. Going forward, all new pages and features must follow the mobile-responsive rule documented in CLAUDE.md.

---

### Other fixes

- `max_chunks` and `min_score` from `agent_connectors` table now respected (were previously hardcoded to 8 / 0.75)
- `min_score` applied as a SQL `WHERE` filter before returning retrieval results
- Retriever keyword fallback: when semantic search returns 0 results (no chunk embeddings), automatically retries with full-text search instead of returning empty
