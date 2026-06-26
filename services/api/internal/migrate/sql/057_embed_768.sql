-- Change connector_chunks embedding dimension from 1536 (OpenAI) to 768 (nomic-embed-text).
-- Safe: all existing embeddings are NULL (never populated), so no data is lost.
DROP INDEX IF EXISTS connector_chunks_embedding_idx;
ALTER TABLE connector_chunks ALTER COLUMN embedding TYPE vector(768);
CREATE INDEX connector_chunks_embedding_idx ON connector_chunks
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
