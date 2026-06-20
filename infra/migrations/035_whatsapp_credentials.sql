-- Persists WhatsApp Baileys auth state across container restarts.
-- The adapter writes all JSON auth files here (encrypted); on startup it reads
-- them back and restores the files before creating the socket, eliminating the
-- need for a QR scan after every Railway deploy.
CREATE TABLE IF NOT EXISTS whatsapp_credentials (
    account_id  TEXT        PRIMARY KEY,
    data        TEXT        NOT NULL,      -- AES-256-GCM encrypted JSON of all auth files
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
