-- name: CreateConversation :one
INSERT INTO conversations (id, subject, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: AddParticipant :exec
INSERT INTO conversation_participants (conversation_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: IsParticipant :one
SELECT EXISTS (
    SELECT 1 FROM conversation_participants
    WHERE conversation_id = $1 AND user_id = $2
);

-- name: GetConversation :one
SELECT * FROM conversations
WHERE id = $1;

-- ListConversations reads the list of one person: the subject, the other
-- people in one string, the time of the newest message, and how many messages
-- this person has not read.
-- name: ListConversations :many
SELECT
    c.id,
    c.subject,
    c.created_by,
    c.created_at,
    COALESCE((
        SELECT string_agg(u.full_name, ', ' ORDER BY u.full_name)
        FROM conversation_participants other
        JOIN users u ON u.id = other.user_id
        WHERE other.conversation_id = c.id AND other.user_id <> p.user_id
    ), '')::text AS others,
    COALESCE((
        SELECT max(m.created_at) FROM messages m WHERE m.conversation_id = c.id
    ), c.created_at)::timestamptz AS last_message_at,
    (
        SELECT count(*)
        FROM messages m
        WHERE m.conversation_id = c.id
          AND m.author_id <> p.user_id
          AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
    )::bigint AS unread
FROM conversations c
JOIN conversation_participants p ON p.conversation_id = c.id
WHERE p.user_id = $1
ORDER BY last_message_at DESC;

-- name: ListParticipants :many
SELECT users.* FROM conversation_participants
JOIN users ON users.id = conversation_participants.user_id
WHERE conversation_participants.conversation_id = $1
ORDER BY users.full_name;

-- name: ListMessages :many
SELECT
    m.id,
    m.author_id,
    u.full_name AS author_name,
    m.body,
    m.created_at
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.conversation_id = $1
ORDER BY m.created_at, m.id;

-- name: CreateMessage :one
INSERT INTO messages (id, conversation_id, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- MarkRead notes that this person has seen the conversation up to now.
-- name: MarkRead :exec
UPDATE conversation_participants
SET last_read_at = now()
WHERE conversation_id = $1 AND user_id = $2;
