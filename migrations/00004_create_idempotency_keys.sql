-- +goose Up
CREATE TABLE idempotency_keys (
    id         UUID        PRIMARY KEY,
    key        VARCHAR(255) NOT NULL,
    scope      VARCHAR(20)  NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT now(),

    UNIQUE (key, scope)
);

-- +goose Down
DROP TABLE idempotency_keys;
