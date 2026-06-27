-- Store WhatsApp LID (Linked Device ID) JIDs on contacts so the pairing policy
-- can match @lid senders to existing contacts even when the phone JID differs.
ALTER TABLE gateway_contacts ADD COLUMN IF NOT EXISTS whatsapp_lid TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS gateway_contacts_channel_lid_uidx
    ON gateway_contacts(channel_id, whatsapp_lid)
    WHERE whatsapp_lid <> '';
