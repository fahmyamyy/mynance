-- +goose Up
-- +goose StatementBegin

-- Extend the role enum to include SYSTEM. The constraint was created in
-- 00008 and must be recreated to add the new value.
ALTER TABLE users DROP CONSTRAINT chk_user_role;
ALTER TABLE users ADD CONSTRAINT chk_user_role
    CHECK (role IN ('USER', 'ADMIN', 'SYSTEM'));

-- Seed the internal MARKET user. Fixed UUID so settlement code can reference
-- it as a constant; safe across environments. Email + username start with an
-- underscore so they can never collide with a real signup
-- (the API rejects those characters at registration).
INSERT INTO users (id, email, username, full_name, password_hash, status, role, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001',
        '_market@internal', '_market', 'Market Maker',
        '', 'ACTIVE', 'SYSTEM', now(), now())
ON CONFLICT (id) DO NOTHING;

-- One account row per enabled asset, owned by the MARKET user. The market's
-- ledger balance is allowed to go arbitrarily negative — it's the
-- counterparty for every sandbox-matched real-user trade.
INSERT INTO accounts (id, user_id, asset, created_at)
SELECT gen_random_uuid(),
       '00000000-0000-0000-0000-000000000001'::uuid,
       symbol,
       now()
FROM assets
WHERE enabled = TRUE
ON CONFLICT (user_id, asset) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM accounts WHERE user_id = '00000000-0000-0000-0000-000000000001'::uuid;
DELETE FROM users    WHERE id      = '00000000-0000-0000-0000-000000000001'::uuid;
ALTER TABLE users DROP CONSTRAINT chk_user_role;
ALTER TABLE users ADD CONSTRAINT chk_user_role CHECK (role IN ('USER', 'ADMIN'));
-- +goose StatementEnd
