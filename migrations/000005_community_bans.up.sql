-- Migration: 000005_community_bans
-- Description: Add community bans table for tracking banned users

CREATE TABLE IF NOT EXISTS community_bans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    banned_by UUID NOT NULL REFERENCES users(id),
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(community_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_community_bans_community_id ON community_bans(community_id);
CREATE INDEX IF NOT EXISTS idx_community_bans_user_id ON community_bans(user_id);
