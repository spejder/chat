-- CreateUser writes one user. The application has no way for a visitor to
-- create a user yet, so the tests and the seed use this query.
-- name: CreateUser :one
INSERT INTO users (id, full_name, email, phone_number)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower(sqlc.arg(email)::text);

-- The identifier is a UUID version 7, which starts with the time, so this
-- order is the order of creation.
-- name: ListUsers :many
SELECT * FROM users
ORDER BY id;

-- SetUserPhone fixes the number of a user. The seed uses it, because a
-- migration cannot know the numbers.
-- name: SetUserPhone :one
UPDATE users
SET phone_number = $2, updated_at = now()
WHERE id = $1
RETURNING *;
