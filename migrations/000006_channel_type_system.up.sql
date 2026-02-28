-- Migration: 000006_channel_type_system
-- Description: Replace the rigid channel_type enum with a flexible, extensible
-- channel type system. Types are now plain strings backed by a definition table
-- so plugins can register their own types without schema changes.

-- Channel type definitions - the registry of all known channel types
CREATE TABLE IF NOT EXISTS channel_type_definitions (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    icon VARCHAR(64) NOT NULL DEFAULT 'hash',
    -- Capabilities this type supports, stored as a bitmask:
    --   1  = messages (text chat)
    --   2  = threads (threaded replies)
    --   4  = media (media-first content)
    --   8  = voice (real-time audio)
    --   16 = video (real-time video)
    --   32 = embeds (rich embeds / link previews)
    --   64 = pins (message pinning)
    --  128 = reactions
    --  256 = slowmode
    --  512 = read-only (only admins can post)
    -- 1024 = topics (topic/thread-starter based)
    capabilities BIGINT NOT NULL DEFAULT 0,
    -- Default metadata template for channels of this type (JSON)
    default_metadata JSONB DEFAULT '{}',
    -- Whether this is a built-in type (can't be deleted)
    built_in BOOLEAN NOT NULL DEFAULT FALSE,
    -- Plugin that registered this type (null for built-in)
    plugin_id VARCHAR(128),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Swap the enum column for a plain varchar so new types don't need ALTER TYPE
ALTER TABLE channels ALTER COLUMN type TYPE VARCHAR(64) USING type::text;

-- Type-specific configuration and state for each channel
ALTER TABLE channels ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}';

-- Seed the built-in channel types
INSERT INTO channel_type_definitions (id, name, description, icon, capabilities, default_metadata, built_in)
VALUES
    ('text', 'Text', 'Send messages, images, and files', 'hash',
     1 | 2 | 32 | 64 | 128 | 256, '{}', true),
    ('announcement', 'Announcement', 'Important updates - only moderators can post', 'megaphone',
     1 | 32 | 64 | 128 | 512, '{}', true),
    ('gallery', 'Gallery', 'Share and browse images and media', 'image',
     1 | 4 | 128, '{"layout": "grid", "columns": 3}', true),
    ('forum', 'Forum', 'Organized discussions with topics and threads', 'messages-square',
     1 | 2 | 32 | 64 | 128 | 1024, '{"defaultSort": "latest", "requireTopic": true}', true),
    ('voice', 'Voice', 'Hang out with real-time voice chat', 'volume-2',
     8 | 16, '{"maxParticipants": 0}', true)
ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_channels_type ON channels(type);
CREATE INDEX IF NOT EXISTS idx_channels_metadata ON channels USING gin(metadata);
