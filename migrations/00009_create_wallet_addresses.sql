-- +goose Up
CREATE TABLE wallet_addresses (
    id         UUID         PRIMARY KEY,
    user_id    UUID         NOT NULL,
    asset      VARCHAR(20)  NOT NULL,
    address    VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP    NOT NULL DEFAULT now(),
    UNIQUE (user_id, asset)
);

CREATE INDEX idx_wallet_user ON wallet_addresses(user_id);

-- +goose Down
DROP TABLE wallet_addresses;
