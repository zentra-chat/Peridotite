ALTER TABLE message_attachments ADD COLUMN IF NOT EXISTS dm_message_id UUID;
ALTER TABLE message_attachments ADD COLUMN IF NOT EXISTS dm_conversation_id UUID;

CREATE INDEX IF NOT EXISTS idx_message_attachments_dm_message_id ON message_attachments(dm_message_id);
CREATE INDEX IF NOT EXISTS idx_message_attachments_dm_conversation_id ON message_attachments(dm_conversation_id);
