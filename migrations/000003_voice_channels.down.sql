-- Migration: 000003_voice_channels (DOWN)
-- Description: Remove voice channel support

DROP TABLE IF EXISTS voice_states;

-- Note: PostgreSQL does not support removing values from enums.
-- The 'voice' value in channel_type will remain but be unused.
