-- Rollback: 000006_channel_type_system
-- Revert channel type back to enum and drop the definitions table.

DROP INDEX IF EXISTS idx_channels_metadata;
DROP INDEX IF EXISTS idx_channels_type;

ALTER TABLE channels DROP COLUMN IF EXISTS metadata;

-- Convert back to enum (only keep values that exist in the original enum)
UPDATE channels SET type = 'text' WHERE type NOT IN ('text', 'announcement', 'gallery', 'forum', 'voice');
ALTER TABLE channels ALTER COLUMN type TYPE channel_type USING type::channel_type;

DROP TABLE IF EXISTS channel_type_definitions;
