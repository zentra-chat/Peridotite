ALTER TABLE messages ADD COLUMN IF NOT EXISTS link_previews JSONB DEFAULT '[]'::jsonb;
ALTER TABLE direct_messages ADD COLUMN IF NOT EXISTS link_previews JSONB DEFAULT '[]'::jsonb;
