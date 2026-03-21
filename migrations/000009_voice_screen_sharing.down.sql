-- Migration: 000009_voice_screen_sharing
-- Description: Remove persisted screen sharing state from voice participants

ALTER TABLE voice_states
DROP COLUMN IF EXISTS is_screen_sharing;
