-- +goose Up
-- A conversation carries a subject and a fixed set of people.
CREATE TABLE conversations (
    id         uuid        PRIMARY KEY,
    subject    text        NOT NULL,
    created_by uuid        NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One row per person in a conversation. last_read_at answers how much of the
-- conversation that person has seen.
CREATE TABLE conversation_participants (
    conversation_id uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    added_at        timestamptz NOT NULL DEFAULT now(),
    last_read_at    timestamptz,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE messages (
    id              uuid        PRIMARY KEY,
    conversation_id uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    author_id       uuid        NOT NULL REFERENCES users (id),
    body            text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- The page reads one conversation in time order, and the unread count reads
-- the same rows.
CREATE INDEX messages_conversation_id_created_at_idx
    ON messages (conversation_id, created_at);

-- +goose Down
DROP TABLE messages;
DROP TABLE conversation_participants;
DROP TABLE conversations;
