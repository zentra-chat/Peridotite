-- Revert plugin system

-- Unlink channel types from core plugin
UPDATE channel_type_definitions SET plugin_id = NULL WHERE built_in = TRUE;

-- Drop indexes
DROP INDEX IF EXISTS idx_plugins_source;
DROP INDEX IF EXISTS idx_plugins_slug;
DROP INDEX IF EXISTS idx_plugin_audit_log_community;
DROP INDEX IF EXISTS idx_plugin_sources_community;
DROP INDEX IF EXISTS idx_community_plugins_plugin;
DROP INDEX IF EXISTS idx_community_plugins_community;

-- Drop tables in dependency order
DROP TABLE IF EXISTS plugin_audit_log;
DROP TABLE IF EXISTS plugin_sources;
DROP TABLE IF EXISTS community_plugins;
DROP TABLE IF EXISTS plugins;
