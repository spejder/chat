-- +goose Up
-- The one VAPID key pair of the site. The push services bind every
-- subscription to the public key, so the pair must never change: a new pair
-- makes every subscription useless. The check keeps the table at one row.
CREATE TABLE vapid_keys (
    id          smallint    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    public_key  text        NOT NULL,
    private_key text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- A browser that wants to hear about new messages. The row belongs to a
-- session, so signing out removes it, and a browser never receives the
-- messages of the person who used it before.
CREATE TABLE push_subscriptions (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_key bytea       NOT NULL REFERENCES sessions (token_hash) ON DELETE CASCADE,
    endpoint    text        NOT NULL UNIQUE,
    p256dh      text        NOT NULL,
    auth        text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX push_subscriptions_user_id_idx ON push_subscriptions (user_id);
CREATE INDEX push_subscriptions_session_key_idx ON push_subscriptions (session_key);

-- +goose Down
DROP TABLE push_subscriptions;
DROP TABLE vapid_keys;
