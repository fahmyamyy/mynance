-- +goose Up
CREATE TABLE ledger_entries (
    id         UUID        PRIMARY KEY,
    user_id    UUID        NOT NULL,
    account_id UUID        NOT NULL,
    asset      VARCHAR(20) NOT NULL,
    amount     NUMERIC(30, 10) NOT NULL,
    entry_type VARCHAR(20) NOT NULL,
    ref_type   VARCHAR(20) NOT NULL,
    ref_id     UUID        NOT NULL,
    created_at TIMESTAMP   NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_user_asset ON ledger_entries(user_id, asset);
CREATE INDEX idx_ledger_ref ON ledger_entries(ref_type, ref_id);

-- +goose Down
DROP TABLE ledger_entries;
