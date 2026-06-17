-- +goose Up
CREATE TABLE deposits (
    id           UUID            PRIMARY KEY,
    user_id      UUID            NOT NULL,
    asset        VARCHAR(20)     NOT NULL,
    address      VARCHAR(255)    NOT NULL,
    amount       NUMERIC(30, 10) NOT NULL,
    tx_hash      VARCHAR(255)    NOT NULL UNIQUE,
    status       VARCHAR(20)     NOT NULL DEFAULT 'PENDING',
    created_at   TIMESTAMP       NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMP,

    CONSTRAINT chk_deposit_status CHECK (status IN ('PENDING', 'CONFIRMED', 'REJECTED'))
);

CREATE INDEX idx_deposits_user ON deposits(user_id);
CREATE INDEX idx_deposits_status ON deposits(status);

-- +goose Down
DROP TABLE deposits;
