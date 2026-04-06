-- Migration: 000010_friend_system
-- Description: Remove friend requests and friendships

DROP INDEX IF EXISTS idx_user_friendships_unique_pair;
DROP INDEX IF EXISTS idx_user_friendships_friend_id;
DROP TABLE IF EXISTS user_friendships;

DROP INDEX IF EXISTS idx_friend_requests_receiver_id;
DROP INDEX IF EXISTS idx_friend_requests_sender_id;
DROP TABLE IF EXISTS friend_requests;
