-- +goose Up
ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'USER';
ALTER TABLE users ADD CONSTRAINT chk_user_role CHECK (role IN ('USER', 'ADMIN'));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT chk_user_role;
ALTER TABLE users DROP COLUMN role;
