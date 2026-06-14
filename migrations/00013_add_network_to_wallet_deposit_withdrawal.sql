-- +goose Up
-- Add network_id to wallet_addresses, deposits, withdrawals.
-- Existing rows back-filled with the first enabled network for the matching asset.

-- wallet_addresses
ALTER TABLE wallet_addresses ADD COLUMN network_id UUID REFERENCES networks(id);

UPDATE wallet_addresses w SET network_id = n.id
FROM (
    SELECT DISTINCT ON (asset_symbol) id, asset_symbol
    FROM networks WHERE enabled = TRUE
    ORDER BY asset_symbol, created_at ASC
) n
WHERE w.asset = n.asset_symbol;

ALTER TABLE wallet_addresses ALTER COLUMN network_id SET NOT NULL;

ALTER TABLE wallet_addresses DROP CONSTRAINT wallet_addresses_user_id_asset_key;
ALTER TABLE wallet_addresses ADD CONSTRAINT wallet_addresses_user_asset_network_unique
    UNIQUE (user_id, asset, network_id);

-- deposits
ALTER TABLE deposits ADD COLUMN network_id UUID REFERENCES networks(id);

UPDATE deposits d SET network_id = n.id
FROM (
    SELECT DISTINCT ON (asset_symbol) id, asset_symbol
    FROM networks WHERE enabled = TRUE
    ORDER BY asset_symbol, created_at ASC
) n
WHERE d.asset = n.asset_symbol;

ALTER TABLE deposits ALTER COLUMN network_id SET NOT NULL;

CREATE INDEX idx_deposits_network ON deposits(network_id);

-- withdrawals
ALTER TABLE withdrawals ADD COLUMN network_id UUID REFERENCES networks(id);

UPDATE withdrawals w SET network_id = n.id
FROM (
    SELECT DISTINCT ON (asset_symbol) id, asset_symbol
    FROM networks WHERE enabled = TRUE
    ORDER BY asset_symbol, created_at ASC
) n
WHERE w.asset = n.asset_symbol;

ALTER TABLE withdrawals ALTER COLUMN network_id SET NOT NULL;

CREATE INDEX idx_withdrawals_network ON withdrawals(network_id);

-- +goose Down
ALTER TABLE withdrawals DROP COLUMN network_id;
ALTER TABLE deposits DROP COLUMN network_id;
ALTER TABLE wallet_addresses DROP CONSTRAINT wallet_addresses_user_asset_network_unique;
ALTER TABLE wallet_addresses ADD CONSTRAINT wallet_addresses_user_id_asset_key UNIQUE (user_id, asset);
ALTER TABLE wallet_addresses DROP COLUMN network_id;
