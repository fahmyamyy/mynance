package trade

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, trade *Trade) error
}

type pgxStore struct {
	db *pgxpool.Pool
}

func NewStore(
	db *pgxpool.Pool,
) Store {
	return &pgxStore{
		db: db,
	}
}

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, t *Trade) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO trades (id, symbol, buy_order_id, sell_order_id, buy_user_id, sell_user_id, price, quantity, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.Symbol, t.BuyOrderID, t.SellOrderID, t.BuyUserID, t.SellUserID, t.Price, t.Quantity, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("tradeStore.Create: %w", err)
	}
	return nil
}
