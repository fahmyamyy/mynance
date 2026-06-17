-- +goose Up
CREATE TABLE orders (
    id              UUID            PRIMARY KEY,
    user_id         UUID            NOT NULL,
    symbol          VARCHAR(20)     NOT NULL,
    side            VARCHAR(4)      NOT NULL,
    price           NUMERIC(30, 10) NOT NULL,
    quantity        NUMERIC(30, 10) NOT NULL,
    filled_quantity NUMERIC(30, 10) NOT NULL DEFAULT 0,
    status          VARCHAR(20)     NOT NULL,
    created_at      TIMESTAMP       NOT NULL DEFAULT now(),
    updated_at      TIMESTAMP       NOT NULL DEFAULT now(),

    CONSTRAINT chk_order_side CHECK (side IN ('BUY', 'SELL')),
    CONSTRAINT chk_order_status CHECK (status IN ('OPEN', 'PARTIAL', 'FILLED', 'CANCELLED'))
);

CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_symbol_status ON orders(symbol, status);

-- +goose Down
DROP TABLE orders;
