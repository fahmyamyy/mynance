package trade

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, trade *Trade) error
	ListByUser(ctx context.Context, userID uuid.UUID, symbol string, limit, offset int) ([]*UserTrade, error)
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

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID, symbol string, limit, offset int) ([]*UserTrade, error) {
	query := `SELECT id, symbol,
	                 CASE WHEN buy_user_id = $1 THEN 'BUY' ELSE 'SELL' END AS side,
	                 price, quantity,
	                 CASE WHEN buy_user_id = $1 THEN sell_user_id ELSE buy_user_id END AS counterparty,
	                 created_at
	          FROM trades
	          WHERE (buy_user_id = $1 OR sell_user_id = $1)`
	args := []any{userID}
	if symbol != "" {
		query += ` AND symbol = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		args = append(args, symbol, limit, offset)
	} else {
		query += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = append(args, limit, offset)
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tradeStore.ListByUser: %w", err)
	}
	defer rows.Close()
	var out []*UserTrade
	for rows.Next() {
		t := &UserTrade{}
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &t.Price, &t.Quantity, &t.CounterpartyUserID, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("tradeStore.ListByUser scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
