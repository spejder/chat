-- ClaimReminders finds every person who missed a message in a conversation
-- and notes the SMS for them in the same statement. A pair is due when:
--
--   1. a message from somebody else is newer than the last reading of the
--      person, and was written between not_before and due_before,
--   2. the person has no browser with a live push subscription,
--   3. the person left the SMS on and has a phone number,
--   4. no SMS for this pair went out after the last reading.
--
-- name: ClaimReminders :many
WITH due AS (
    SELECT p.user_id, p.conversation_id
    FROM conversation_participants p
    JOIN users u ON u.id = p.user_id
    WHERE u.sms_reminders
      AND u.phone_number <> ''
      AND EXISTS (
          SELECT 1 FROM messages m
          WHERE m.conversation_id = p.conversation_id
            AND m.author_id <> p.user_id
            AND m.created_at > coalesce(p.last_read_at, '-infinity'::timestamptz)
            AND m.created_at > @not_before::timestamptz
            AND m.created_at <= @due_before::timestamptz
      )
      AND NOT EXISTS (
          SELECT 1 FROM push_subscriptions ps
          JOIN sessions s ON s.token_hash = ps.session_key
          WHERE ps.user_id = p.user_id AND s.expires_at > now()
      )
      AND NOT EXISTS (
          SELECT 1 FROM sms_reminders r
          WHERE r.user_id = p.user_id
            AND r.conversation_id = p.conversation_id
            AND r.sent_at > coalesce(p.last_read_at, '-infinity'::timestamptz)
      )
),
claimed AS (
    INSERT INTO sms_reminders (user_id, conversation_id, sent_at)
    SELECT user_id, conversation_id, now() FROM due
    ON CONFLICT (user_id, conversation_id) DO UPDATE SET sent_at = excluded.sent_at
    RETURNING user_id, conversation_id
)
SELECT claimed.user_id, claimed.conversation_id, u.phone_number, c.subject
FROM claimed
JOIN users u ON u.id = claimed.user_id
JOIN conversations c ON c.id = claimed.conversation_id
ORDER BY claimed.user_id, claimed.conversation_id;

-- LockReminders lets one server instance claim at a time. The lock ends
-- with the transaction.
-- name: LockReminders :one
SELECT pg_try_advisory_xact_lock(hashtext('sms_reminders'))::boolean AS locked;
