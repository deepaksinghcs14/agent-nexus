-- Add required_tool_names to skills so enabling a skill auto-attaches its tools to the agent.
ALTER TABLE skills ADD COLUMN IF NOT EXISTS required_tool_names TEXT[] NOT NULL DEFAULT '{}';

UPDATE skills SET required_tool_names = ARRAY['native_save_memory']
WHERE workspace_id IS NULL AND name = 'Contextual Learning';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_get_current_context']
WHERE workspace_id IS NULL AND name = 'WhatsApp Identity Verification';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_search_contacts','whatsapp_send_message']
WHERE workspace_id IS NULL AND name = 'WhatsApp Messaging Operator';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_search_contacts']
WHERE workspace_id IS NULL AND name = 'WhatsApp Contact Resolver';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_send_message']
WHERE workspace_id IS NULL AND name = 'WhatsApp Raw Number Sender';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_create_reminder','whatsapp_list_reminders','whatsapp_complete_reminder']
WHERE workspace_id IS NULL AND name = 'WhatsApp Follow-up Scheduler';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_send_media_status']
WHERE workspace_id IS NULL AND name = 'WhatsApp Media Handler';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_summarize_link']
WHERE workspace_id IS NULL AND name = 'WhatsApp Link Summarizer';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_request_owner_approval']
WHERE workspace_id IS NULL AND name = 'WhatsApp Safety & Consent';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_request_owner_approval']
WHERE workspace_id IS NULL AND name = 'WhatsApp Owner Escalation';

UPDATE skills SET required_tool_names = ARRAY['whatsapp_send_message','whatsapp_search_contacts','whatsapp_create_reminder','whatsapp_summarize_link','whatsapp_request_owner_approval']
WHERE workspace_id IS NULL AND name = 'WhatsApp Personal Assistant';
