-- +goose Up
CREATE TABLE trades (
    id            UUID            PRIMARY KEY,
    symbol        VARCHAR(20)     NOT NULL,
    buy_order_id  UUID            NOT NULL,
    sell_order_id UUID            NOT NULL,
    buy_user_id   UUID            NOT NULL,
    sell_user_id  UUID            NOT NULL,
    price         NUMERIC(30, 10) NOT NULL,
    quantity      NUMERIC(30, 10) NOT NULL,
    created_at    TIMESTAMP       NOT NULL DEFAULT now()
);

CREATE INDEX idx_trades_symbol ON trades(symbol);
CREATE INDEX idx_trades_user_buy ON trades(buy_user_id);
CREATE INDEX idx_trades_user_sell ON trades(sell_user_id);

-- +goose Down
DROP TABLE trades;
