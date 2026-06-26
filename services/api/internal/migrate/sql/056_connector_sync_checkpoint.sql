-- Add checkpoint storage to sync jobs so a sync can resume from where it left off after a pod restart.
ALTER TABLE connector_sync_jobs
    ADD COLUMN IF NOT EXISTS checkpoint JSONB NOT NULL DEFAULT '{}';

-- Fix missing ON DELETE CASCADE on context_retrieval_logs.
-- Without it, deleting a connector (or its documents/chunks) would fail with an FK violation
-- if the connector had ever been used for context retrieval.
ALTER TABLE context_retrieval_logs
    DROP CONSTRAINT IF EXISTS context_retrieval_logs_connector_id_fkey,
    DROP CONSTRAINT IF EXISTS context_retrieval_logs_document_id_fkey,
    DROP CONSTRAINT IF EXISTS context_retrieval_logs_chunk_id_fkey;

ALTER TABLE context_retrieval_logs
    ADD CONSTRAINT context_retrieval_logs_connector_id_fkey
        FOREIGN KEY (connector_id) REFERENCES connectors(id) ON DELETE CASCADE,
    ADD CONSTRAINT context_retrieval_logs_document_id_fkey
        FOREIGN KEY (document_id) REFERENCES connector_documents(id) ON DELETE CASCADE,
    ADD CONSTRAINT context_retrieval_logs_chunk_id_fkey
        FOREIGN KEY (chunk_id) REFERENCES connector_chunks(id) ON DELETE CASCADE;
