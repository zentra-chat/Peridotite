-- Migration: 000002_notifications (down)
DROP INDEX IF EXISTS idx_message_mentions_community_id;
DROP INDEX IF EXISTS idx_message_mentions_channel_id;
DROP INDEX IF EXISTS idx_message_mentions_mentioned_user;
DROP INDEX IF EXISTS idx_message_mentions_message_id;
DROP TABLE IF EXISTS message_mentions;

DROP INDEX IF EXISTS idx_notifications_actor_id;
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_user_id;
DROP TABLE IF EXISTS notifications;
