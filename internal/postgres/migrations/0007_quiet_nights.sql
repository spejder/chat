-- +goose Up
-- A person may let notifications sound and SMS reminders go out in the
-- night. Everybody starts with quiet nights.
ALTER TABLE users ADD COLUMN quiet_nights boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE users DROP COLUMN quiet_nights;
