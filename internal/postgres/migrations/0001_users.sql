-- +goose Up
CREATE TABLE users (
    id         uuid        PRIMARY KEY,
    full_name  text        NOT NULL,
    email      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- The index reads the address in lower case, so one address cannot arrive
-- twice in different case.
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

-- +goose Down
DROP TABLE users;
