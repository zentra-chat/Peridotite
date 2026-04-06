-- Migration: 000011_webhooks
-- Description: Add webhook integrations for channel message delivery

CREATE TABLE IF NOT EXISTS webhooks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bot_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name VARCHAR(80) NOT NULL,
    avatar_url TEXT,
    provider_hint VARCHAR(32),
    token_hash VARCHAR(128) NOT NULL,
    token_preview VARCHAR(24) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webhooks_bot_user_id ON webhooks(bot_user_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_channel_id ON webhooks(channel_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_community_id ON webhooks(community_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_created_by ON webhooks(created_by);
CREATE INDEX IF NOT EXISTS idx_webhooks_lookup ON webhooks(id, token_hash) WHERE is_active = TRUE;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_webhooks_updated_at') THEN
    CREATE TRIGGER update_webhooks_updated_at BEFORE UPDATE ON webhooks
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
END IF; END $$;
