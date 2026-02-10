DROP INDEX IF EXISTS idx_message_attachments_dm_conversation_id;
DROP INDEX IF EXISTS idx_message_attachments_dm_message_id;

ALTER TABLE message_attachments DROP COLUMN IF EXISTS dm_conversation_id;
ALTER TABLE message_attachments DROP COLUMN IF EXISTS dm_message_id;
