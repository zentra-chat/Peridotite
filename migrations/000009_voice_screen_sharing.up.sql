-- Migration: 000009_voice_screen_sharing
-- Description: Add persisted screen sharing state to voice participants

ALTER TABLE voice_states
ADD COLUMN IF NOT EXISTS is_screen_sharing BOOLEAN NOT NULL DEFAULT FALSE;
