-- +goose Up
CREATE TABLE users (
    id            UUID        PRIMARY KEY,
    email         VARCHAR(255) NOT NULL,
    username      VARCHAR(100) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',
    deleted_at    TIMESTAMP,
    created_at    TIMESTAMP    NOT NULL DEFAULT now(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT now(),

    UNIQUE (email),
    UNIQUE (username),
    CONSTRAINT chk_user_status CHECK (status IN ('ACTIVE', 'SUSPENDED', 'CLOSED'))
);

-- +goose Down
DROP TABLE users;
