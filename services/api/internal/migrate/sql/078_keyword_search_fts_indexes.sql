-- The keyword-search leg of hybrid RAG matched on
-- to_tsvector(cc.content || ' ' || cd.title) — a two-table expression no
-- index can cover, so every query seq-scanned connector_chunks. The
-- predicate is now two single-table disjuncts; these are the matching
-- functional indexes. Ranking still uses the concatenated expression, but
-- only over the already-filtered rows.
-- Plain CREATE INDEX, not CONCURRENTLY: migrate.go runs each file in one
-- transaction, and CONCURRENTLY cannot run inside one.
CREATE INDEX IF NOT EXISTS connector_chunks_content_fts_idx
    ON connector_chunks USING GIN (to_tsvector('english', content));

CREATE INDEX IF NOT EXISTS connector_documents_title_fts_idx
    ON connector_documents USING GIN (to_tsvector('english', title));
