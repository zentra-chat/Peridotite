DROP INDEX IF EXISTS idx_direct_messages_reply_to_id;

ALTER TABLE direct_messages DROP COLUMN IF EXISTS reactions;
ALTER TABLE direct_messages DROP COLUMN IF EXISTS reply_to_id;
