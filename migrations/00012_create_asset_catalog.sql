-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

CREATE TABLE assets (
    symbol         VARCHAR(20)     PRIMARY KEY,
    name           VARCHAR(100)    NOT NULL,
    decimals       INT             NOT NULL DEFAULT 8,
    min_deposit    NUMERIC(30, 10) NOT NULL DEFAULT 0,
    min_withdrawal NUMERIC(30, 10) NOT NULL DEFAULT 0,
    enabled        BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMP       NOT NULL DEFAULT now()
);

CREATE TABLE networks (
    id                UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_symbol      VARCHAR(20)     NOT NULL REFERENCES assets(symbol) ON DELETE CASCADE,
    name              VARCHAR(50)     NOT NULL,
    chain_id          VARCHAR(50),
    address_pattern   VARCHAR(200),
    withdrawal_fee    NUMERIC(30, 10) NOT NULL DEFAULT 0,
    min_confirmations INT             NOT NULL DEFAULT 1,
    enabled           BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMP       NOT NULL DEFAULT now(),
    UNIQUE (asset_symbol, name)
);

CREATE INDEX idx_networks_asset ON networks(asset_symbol);

INSERT INTO assets (symbol, name, decimals, min_deposit, min_withdrawal) VALUES
    ('BTC',  'Bitcoin',  8, 0.0001,   0.0005),
    ('ETH',  'Ethereum', 18, 0.001,   0.005),
    ('USDT', 'Tether',   6, 1,        10),
    ('USDC', 'USD Coin', 6, 1,        10),
    ('SOL',  'Solana',   9, 0.01,     0.05);

INSERT INTO networks (asset_symbol, name, chain_id, address_pattern, withdrawal_fee, min_confirmations) VALUES
    ('BTC',  'Bitcoin', 'bitcoin', '^(bc1|[13])[a-zA-HJ-NP-Z0-9]{25,62}$', 0.0001, 3),
    ('ETH',  'ERC20',   '1',       '^0x[a-fA-F0-9]{40}$',                  0.003,  12),
    ('USDT', 'ERC20',   '1',       '^0x[a-fA-F0-9]{40}$',                  5,      12),
    ('USDT', 'BEP20',   '56',      '^0x[a-fA-F0-9]{40}$',                  1,      15),
    ('USDT', 'TRON',    'tron',    '^T[a-zA-Z0-9]{33}$',                   1,      20),
    ('USDC', 'ERC20',   '1',       '^0x[a-fA-F0-9]{40}$',                  5,      12),
    ('USDC', 'BEP20',   '56',      '^0x[a-fA-F0-9]{40}$',                  1,      15),
    ('SOL',  'Solana',  'solana',  '^[1-9A-HJ-NP-Za-km-z]{32,44}$',        0.01,   1);

-- +goose Down
DROP TABLE networks;
DROP TABLE assets;
