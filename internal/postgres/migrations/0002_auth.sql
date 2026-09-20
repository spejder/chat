-- +goose Up
-- The column arrives with a default so that the rows that are already there
-- get a value. The default goes away again, so every later row must bring a
-- number.
ALTER TABLE users ADD COLUMN phone_number text NOT NULL DEFAULT '';
ALTER TABLE users ALTER COLUMN phone_number DROP DEFAULT;

-- A passkey. The library owns the shape of the credential, so the row keeps
-- it as JSON and only lifts out what a lookup needs.
CREATE TABLE webauthn_credentials (
    credential_id bytea       PRIMARY KEY,
    user_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    data          jsonb       NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz
);

CREATE INDEX webauthn_credentials_user_id_idx ON webauthn_credentials (user_id);

-- The state between the two halves of a passkey ceremony. The browser holds
-- only the identifier, in a short lived cookie.
CREATE TABLE webauthn_challenges (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text        NOT NULL,
    data       jsonb       NOT NULL,
    expires_at timestamptz NOT NULL
);

-- A code that went out by SMS. The row keeps the hash, never the code.
CREATE TABLE otp_codes (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash   bytea       NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX otp_codes_user_id_idx ON otp_codes (user_id);

-- A signed in browser. The cookie holds the token, the row holds its hash.
CREATE TABLE sessions (
    token_hash bytea       PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
DROP TABLE otp_codes;
DROP TABLE webauthn_challenges;
DROP TABLE webauthn_credentials;
ALTER TABLE users DROP COLUMN phone_number;
