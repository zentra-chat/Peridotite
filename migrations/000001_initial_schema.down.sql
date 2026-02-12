-- Migration: 000001_initial_schema (DOWN)
-- Description: Drop all tables and types

-- Drop triggers
DROP TRIGGER IF EXISTS update_member_count_trigger ON community_members;
DROP TRIGGER IF EXISTS update_user_settings_updated_at ON user_settings;
DROP TRIGGER IF EXISTS update_dm_conversations_updated_at ON dm_conversations;
DROP TRIGGER IF EXISTS update_roles_updated_at ON roles;
DROP TRIGGER IF EXISTS update_channels_updated_at ON channels;
DROP TRIGGER IF EXISTS update_communities_updated_at ON communities;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- Drop functions
DROP FUNCTION IF EXISTS update_community_member_count();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse order of dependencies
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS user_blocks;
DROP TABLE IF EXISTS direct_messages;
DROP TABLE IF EXISTS dm_participants;
DROP TABLE IF EXISTS dm_conversations;
DROP TABLE IF EXISTS message_attachments;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS channel_permissions;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS channel_categories;
DROP TABLE IF EXISTS member_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS community_invites;
DROP TABLE IF EXISTS community_members;
DROP TABLE IF EXISTS communities;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;

-- Drop custom types
DROP TYPE IF EXISTS channel_type;
DROP TYPE IF EXISTS user_status;
