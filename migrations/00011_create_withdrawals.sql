-- +goose Up
CREATE TABLE withdrawals (
    id              UUID            PRIMARY KEY,
    user_id         UUID            NOT NULL,
    account_id      UUID            NOT NULL,
    asset           VARCHAR(20)     NOT NULL,
    amount          NUMERIC(30, 10) NOT NULL,
    status          VARCHAR(20)     NOT NULL DEFAULT 'COMPLETED',
    idempotency_key VARCHAR(255)    NOT NULL,
    created_at      TIMESTAMP       NOT NULL DEFAULT now(),

    CONSTRAINT chk_withdrawal_status CHECK (status IN ('COMPLETED', 'PROCESSING'))
);

CREATE INDEX idx_withdrawals_user ON withdrawals(user_id);

-- +goose Down
DROP TABLE withdrawals;
