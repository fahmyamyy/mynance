package order

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, order *Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error)
	UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status Status) error
	IncrementFilled(ctx context.Context, tx pgx.Tx, id uuid.UUID, qty pgtype.Numeric) error
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

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, o *Order) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO orders (id, user_id, symbol, side, price, quantity, filled_quantity, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		o.ID, o.UserID, o.Symbol, o.Side, o.Price, o.Quantity, o.FilledQuantity, o.Status, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("orderStore.Create: %w", err)
	}
	return nil
}

func scanOrder(scanner interface {
	Scan(dest ...any) error
}) (*Order, error) {
	o := &Order{}
	err := scanner.Scan(
		&o.ID, &o.UserID, &o.Symbol, &o.Side, &o.Price,
		&o.Quantity, &o.FilledQuantity, &o.Status, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *pgxStore) GetByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, user_id, symbol, side, price, quantity, filled_quantity, status, created_at, updated_at
		 FROM orders WHERE id = $1`,
		id,
	)
	o, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("orderStore.GetByID: %w", err)
	}
	return o, nil
}

func (r *pgxStore) GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Order, error) {
	row := tx.QueryRow(ctx,
		`SELECT id, user_id, symbol, side, price, quantity, filled_quantity, status, created_at, updated_at
		 FROM orders WHERE id = $1 FOR UPDATE`,
		id,
	)
	o, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("orderStore.GetByIDTx: %w", err)
	}
	return o, nil
}

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, symbol, side, price, quantity, filled_quantity, status, created_at, updated_at
		 FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("orderStore.ListByUser: %w", err)
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("orderStore.ListByUser scan: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (r *pgxStore) UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status Status) error {
	tag, err := tx.Exec(ctx,
		`UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("orderStore.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *pgxStore) IncrementFilled(ctx context.Context, tx pgx.Tx, id uuid.UUID, qty pgtype.Numeric) error {
	tag, err := tx.Exec(ctx,
		`UPDATE orders SET filled_quantity = filled_quantity + $1, updated_at = now() WHERE id = $2`,
		qty, id,
	)
	if err != nil {
		return fmt.Errorf("orderStore.IncrementFilled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}
