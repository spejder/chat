-- name: CreateCredential :exec
INSERT INTO webauthn_credentials (credential_id, user_id, data)
VALUES ($1, $2, $3);

-- name: ListCredentials :many
SELECT * FROM webauthn_credentials
WHERE user_id = $1
ORDER BY created_at;

-- name: CountCredentials :one
SELECT count(*) FROM webauthn_credentials
WHERE user_id = $1;

-- name: UpdateCredential :exec
UPDATE webauthn_credentials
SET data = $2, last_used_at = now()
WHERE credential_id = $1;

-- name: CreateChallenge :exec
INSERT INTO webauthn_challenges (id, user_id, purpose, data, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: TakeChallenge :one
DELETE FROM webauthn_challenges
WHERE id = $1 AND purpose = $2 AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredChallenges :exec
DELETE FROM webauthn_challenges
WHERE expires_at <= now();

-- ConsumeCodes makes every code of one user useless, which happens as soon as
-- a new code goes out.
-- name: ConsumeCodes :exec
UPDATE otp_codes
SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL;

-- name: CreateCode :exec
INSERT INTO otp_codes (id, user_id, code_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetLatestCode :one
SELECT * FROM otp_codes
WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: CountCodeAttempt :one
UPDATE otp_codes
SET attempts = attempts + 1
WHERE id = $1
RETURNING attempts;

-- name: ConsumeCode :exec
UPDATE otp_codes
SET consumed_at = now()
WHERE id = $1;

-- name: DeleteExpiredCodes :exec
DELETE FROM otp_codes
WHERE expires_at <= now();

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- GetSessionUser reads the person behind a live session in one step.
-- name: GetSessionUser :one
SELECT users.* FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = $1 AND sessions.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= now();

-- CreateSignInLink stores a link. Go writes the nil UUID for a link to the
-- list, and the row holds NULL, because the column refers to a
-- conversation.
-- name: CreateSignInLink :exec
INSERT INTO sign_in_links (token_hash, user_id, conversation_id, expires_at)
VALUES (
    @token_hash,
    @user_id,
    NULLIF(@conversation_id::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
    @expires_at
);

-- GetSignInLink reads a live link. A link works more than once until it
-- expires, because a messaging app may open it for a preview first. A link
-- to the list returns the nil UUID.
-- name: GetSignInLink :one
SELECT
    user_id,
    coalesce(conversation_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS conversation_id
FROM sign_in_links
WHERE token_hash = $1 AND expires_at > now();

-- name: DeleteExpiredSignInLinks :exec
DELETE FROM sign_in_links
WHERE expires_at <= now();
