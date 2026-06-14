-- +goose Up
ALTER TABLE withdrawals ADD COLUMN destination_address VARCHAR(255);

-- Back-fill existing rows with a placeholder so the NOT NULL constraint can be applied.
UPDATE withdrawals SET destination_address = 'legacy-no-address' WHERE destination_address IS NULL;

ALTER TABLE withdrawals ALTER COLUMN destination_address SET NOT NULL;

-- +goose Down
ALTER TABLE withdrawals DROP COLUMN destination_address;
