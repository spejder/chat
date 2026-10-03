-- name: GetVAPIDKeys :one
SELECT public_key, private_key FROM vapid_keys
WHERE id = 1;

-- InsertVAPIDKeys writes a pair unless one is already there. Two instances
-- that start together both read the pair afterwards, so they agree.
-- name: InsertVAPIDKeys :exec
INSERT INTO vapid_keys (id, public_key, private_key)
VALUES (1, $1, $2)
ON CONFLICT (id) DO NOTHING;

-- UpsertSubscription stores a browser. A browser that sends the same endpoint
-- again moves to the person and the session that send it now.
-- name: UpsertSubscription :exec
INSERT INTO push_subscriptions (id, user_id, session_key, endpoint, p256dh, auth)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (endpoint) DO UPDATE
SET user_id     = excluded.user_id,
    session_key = excluded.session_key,
    p256dh      = excluded.p256dh,
    auth        = excluded.auth;

-- name: DeleteSubscription :exec
DELETE FROM push_subscriptions
WHERE endpoint = $1 AND user_id = $2;

-- DeleteSubscriptionByEndpoint removes a browser that the push service no
-- longer knows.
-- name: DeleteSubscriptionByEndpoint :exec
DELETE FROM push_subscriptions
WHERE endpoint = $1;

-- ListSubscriptionsForUsers reads the browsers of some people. A browser
-- whose session ran out hears nothing, although its row is still there.
-- name: ListSubscriptionsForUsers :many
SELECT ps.user_id, ps.endpoint, ps.p256dh, ps.auth
FROM push_subscriptions ps
JOIN sessions s ON s.token_hash = ps.session_key
WHERE ps.user_id = ANY(@user_ids::uuid[])
  AND s.expires_at > now()
ORDER BY ps.created_at;

-- CountUnread counts the messages from other people that each person has not
-- read, over every conversation of that person. The rule is the one of the
-- unread count in ListConversations. The badge on the icon of the installed
-- app shows the number.
-- name: CountUnread :many
SELECT p.user_id, count(m.id)::bigint AS unread
FROM conversation_participants p
LEFT JOIN messages m
    ON m.conversation_id = p.conversation_id
   AND m.author_id <> p.user_id
   AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
WHERE p.user_id = ANY(@user_ids::uuid[])
GROUP BY p.user_id;
