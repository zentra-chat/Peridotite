-- Migration: 000003_voice_channels
-- Description: Add voice channel support

-- Add 'voice' to the channel_type enum
ALTER TYPE channel_type ADD VALUE IF NOT EXISTS 'voice';

-- Voice states - tracks who is currently in a voice channel
CREATE TABLE IF NOT EXISTS voice_states (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_muted BOOLEAN DEFAULT FALSE,
    is_deafened BOOLEAN DEFAULT FALSE,
    is_self_muted BOOLEAN DEFAULT FALSE,
    is_self_deafened BOOLEAN DEFAULT FALSE,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_voice_states_channel_id ON voice_states(channel_id);
CREATE INDEX IF NOT EXISTS idx_voice_states_user_id ON voice_states(user_id);
