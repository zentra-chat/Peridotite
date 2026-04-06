-- Migration: 000010_friend_system
-- Description: Add friend requests and friendships

CREATE TABLE IF NOT EXISTS friend_requests (
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sender_id, receiver_id),
    CONSTRAINT friend_requests_no_self CHECK (sender_id <> receiver_id)
);

CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver_id ON friend_requests(receiver_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_sender_id ON friend_requests(sender_id);

CREATE TABLE IF NOT EXISTS user_friendships (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id),
    CONSTRAINT user_friendships_no_self CHECK (user_id <> friend_id)
);

CREATE INDEX IF NOT EXISTS idx_user_friendships_friend_id ON user_friendships(friend_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_friendships_unique_pair
ON user_friendships (LEAST(user_id, friend_id), GREATEST(user_id, friend_id));
