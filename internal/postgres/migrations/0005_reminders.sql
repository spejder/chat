-- +goose Up
-- A person may turn the SMS about missed messages off. Everybody starts
-- with it on.
ALTER TABLE users ADD COLUMN sms_reminders boolean NOT NULL DEFAULT true;

-- The last SMS about missed messages for one person in one conversation.
-- The next SMS waits until the person read the conversation after this
-- time, so one unread stretch costs one SMS at most.
CREATE TABLE sms_reminders (
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    conversation_id uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    sent_at         timestamptz NOT NULL,
    PRIMARY KEY (user_id, conversation_id)
);

-- A link in an SMS that signs a person in and opens one conversation. The
-- SMS holds the token, the row holds its hash.
CREATE TABLE sign_in_links (
    token_hash      bytea       PRIMARY KEY,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    conversation_id uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sign_in_links_expires_at_idx ON sign_in_links (expires_at);

-- +goose Down
DROP TABLE sign_in_links;
DROP TABLE sms_reminders;
ALTER TABLE users DROP COLUMN sms_reminders;
