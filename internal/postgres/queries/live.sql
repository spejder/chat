-- Notify tells every server instance that a conversation changed. Each one
-- listens on the channel chat_changed and hands the event to its open pages.
-- name: Notify :exec
SELECT pg_notify('chat_changed', @payload::text);
