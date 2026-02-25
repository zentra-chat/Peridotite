-- Migration: 000002_notifications
-- Description: Add notifications and message mentions tables

-- Notifications table
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    community_id UUID REFERENCES communities(id) ON DELETE CASCADE,
    channel_id UUID REFERENCES channels(id) ON DELETE SET NULL,
    message_id UUID,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    metadata JSONB DEFAULT '{}',
    is_read BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id) WHERE is_read = FALSE;
CREATE INDEX IF NOT EXISTS idx_notifications_actor_id ON notifications(actor_id);

-- Message mentions table (tracks who was mentioned in each message)
CREATE TABLE IF NOT EXISTS message_mentions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID NOT NULL,
    message_created_at TIMESTAMPTZ NOT NULL,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    community_id UUID REFERENCES communities(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mentioned_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    mentioned_role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    mention_type VARCHAR(16) NOT NULL CHECK (mention_type IN ('user', 'role', 'everyone', 'here')),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_mentions_message_id ON message_mentions(message_id);
CREATE INDEX IF NOT EXISTS idx_message_mentions_mentioned_user ON message_mentions(mentioned_user_id);
CREATE INDEX IF NOT EXISTS idx_message_mentions_channel_id ON message_mentions(channel_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_mentions_community_id ON message_mentions(community_id);
