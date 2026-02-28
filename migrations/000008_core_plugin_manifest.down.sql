-- Revert core plugin manifest fields to previous defaults
UPDATE plugins
SET
    manifest = jsonb_build_object(
        'channelTypes', jsonb_build_array('text', 'announcement', 'gallery', 'forum', 'voice'),
        'commands', jsonb_build_array(),
        'triggers', jsonb_build_array(),
        'hooks', jsonb_build_array('message.create', 'message.update', 'message.delete', 'member.join', 'member.leave')
    ),
    updated_at = NOW()
WHERE slug = 'core';
