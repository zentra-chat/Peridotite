ALTER TABLE direct_messages ADD COLUMN IF NOT EXISTS reply_to_id UUID;
ALTER TABLE direct_messages ADD COLUMN IF NOT EXISTS reactions JSONB DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_direct_messages_reply_to_id ON direct_messages(reply_to_id);
