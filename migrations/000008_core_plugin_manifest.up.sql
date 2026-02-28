-- Refresh the built-in core plugin metadata to point at the default-plugin repository
UPDATE plugins
SET
    name = 'Zentra Core',
    description = 'Built-in plugin providing the default channel types and base features. This plugin is always active on every server.',
    author = 'Zentra',
    version = '1.0.0',
    source_url = 'default-plugin',
    requested_permissions = ~0::BIGINT,
    manifest = jsonb_build_object(
        'channelTypes', jsonb_build_array('text', 'announcement', 'gallery', 'forum', 'voice'),
        'commands', jsonb_build_array(),
        'triggers', jsonb_build_array(),
        'hooks', jsonb_build_array('channel_registry'),
        'frontendBundle', ''
    ),
    built_in = TRUE,
    source = 'official',
    is_verified = TRUE,
    updated_at = NOW()
WHERE slug = 'core';
