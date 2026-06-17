-- +goose Up
CREATE TABLE accounts (
    id         UUID        PRIMARY KEY,
    user_id    UUID        NOT NULL,
    asset      VARCHAR(20) NOT NULL,
    created_at TIMESTAMP   NOT NULL DEFAULT now(),

    UNIQUE (user_id, asset)
);

-- +goose Down
DROP TABLE accounts;
