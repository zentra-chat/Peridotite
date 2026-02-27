-- Migration: 000004_custom_emojis
-- Description: Add custom emoji support per community

CREATE TABLE IF NOT EXISTS custom_emojis (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    name VARCHAR(32) NOT NULL,
    image_url TEXT NOT NULL,
    uploader_id UUID NOT NULL REFERENCES users(id),
    animated BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(community_id, name)
);

CREATE INDEX IF NOT EXISTS idx_custom_emojis_community_id ON custom_emojis(community_id);
CREATE INDEX IF NOT EXISTS idx_custom_emojis_name ON custom_emojis(name);

-- Trigger for updated_at
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_custom_emojis_updated_at') THEN
    CREATE TRIGGER update_custom_emojis_updated_at BEFORE UPDATE ON custom_emojis
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
END IF; END $$;
