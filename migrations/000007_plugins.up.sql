-- Plugin system tables

-- Stores plugin definitions that have been published/available
CREATE TABLE IF NOT EXISTS plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(256) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author VARCHAR(256) NOT NULL,
    version VARCHAR(64) NOT NULL DEFAULT '0.1.0',
    homepage_url TEXT,
    source_url TEXT,
    icon_url TEXT,
    -- permissions the plugin requests (bitmask)
    requested_permissions BIGINT NOT NULL DEFAULT 0,
    -- the manifest JSON blob stores everything the plugin declares
    manifest JSONB NOT NULL DEFAULT '{}',
    -- whether this is a built-in plugin that ships with every server
    built_in BOOLEAN NOT NULL DEFAULT FALSE,
    -- origin source URL this plugin was fetched from (for apt-style sources)
    source VARCHAR(512) NOT NULL DEFAULT 'official',
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Which plugins are installed on which communities
CREATE TABLE IF NOT EXISTS community_plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    -- whether the plugin is actively running
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    -- permissions the server owner actually granted (subset of requested)
    granted_permissions BIGINT NOT NULL DEFAULT 0,
    -- per-community config overrides for this plugin
    config JSONB NOT NULL DEFAULT '{}',
    installed_by UUID NOT NULL REFERENCES users(id),
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(community_id, plugin_id)
);

-- Plugin sources - apt-style repos people can add
CREATE TABLE IF NOT EXISTS plugin_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    url TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    added_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(community_id, url)
);

-- Audit trail for plugin actions (installs, config changes, etc)
CREATE TABLE IF NOT EXISTS plugin_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(64) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_community_plugins_community ON community_plugins(community_id);
CREATE INDEX IF NOT EXISTS idx_community_plugins_plugin ON community_plugins(plugin_id);
CREATE INDEX IF NOT EXISTS idx_plugin_sources_community ON plugin_sources(community_id);
CREATE INDEX IF NOT EXISTS idx_plugin_audit_log_community ON plugin_audit_log(community_id);
CREATE INDEX IF NOT EXISTS idx_plugins_slug ON plugins(slug);
CREATE INDEX IF NOT EXISTS idx_plugins_source ON plugins(source);

-- Seed the built-in "core" plugin that provides default channel types
INSERT INTO plugins (slug, name, description, author, version, requested_permissions, manifest, built_in, source, is_verified)
VALUES (
    'core',
    'Zentra Core',
    'Built-in plugin providing the default channel types and base features. This plugin is always active on every server.',
    'Zentra',
    '1.0.0',
    -- all permissions since it's the core
    ~0::BIGINT,
    '{
        "channelTypes": ["text", "announcement", "gallery", "forum", "voice"],
        "commands": [],
        "triggers": [],
        "hooks": ["message.create", "message.update", "message.delete", "member.join", "member.leave"]
    }'::JSONB,
    TRUE,
    'official',
    TRUE
) ON CONFLICT (slug) DO NOTHING;

-- Link the existing channel_type_definitions to the core plugin
UPDATE channel_type_definitions
SET plugin_id = (SELECT id::TEXT FROM plugins WHERE slug = 'core')
WHERE built_in = TRUE AND plugin_id IS NULL;
