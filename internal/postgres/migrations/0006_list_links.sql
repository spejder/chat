-- +goose Up
-- A link in an SMS about several conversations opens the list, so it
-- belongs to no conversation.
ALTER TABLE sign_in_links ALTER COLUMN conversation_id DROP NOT NULL;

-- +goose Down
DELETE FROM sign_in_links WHERE conversation_id IS NULL;
ALTER TABLE sign_in_links ALTER COLUMN conversation_id SET NOT NULL;
