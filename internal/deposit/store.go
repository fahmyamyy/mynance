package deposit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, d *Deposit) error
	GetByID(ctx context.Context, id uuid.UUID) (*Deposit, error)
	GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Deposit, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Deposit, error)
	UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status Status, confirmedAt *time.Time) error
}

type pgxStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgxStore{db: db} }

const cols = `id, user_id, asset, network_id, address, amount, tx_hash, status, created_at, confirmed_at`

func scan(s interface{ Scan(...any) error }) (*Deposit, error) {
	d := &Deposit{}
	if err := s.Scan(&d.ID, &d.UserID, &d.Asset, &d.NetworkID, &d.Address, &d.Amount, &d.TxHash, &d.Status, &d.CreatedAt, &d.ConfirmedAt); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, d *Deposit) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO deposits (id, user_id, asset, network_id, address, amount, tx_hash, status, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		d.ID, d.UserID, d.Asset, d.NetworkID, d.Address, d.Amount, d.TxHash, d.Status, d.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return shared.ErrConflict
		}
		return fmt.Errorf("depositStore.Create: %w", err)
	}
	return nil
}

func (r *pgxStore) GetByID(ctx context.Context, id uuid.UUID) (*Deposit, error) {
	row := r.db.QueryRow(ctx, `SELECT `+cols+` FROM deposits WHERE id = $1`, id)
	d, err := scan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("depositStore.GetByID: %w", err)
	}
	return d, nil
}

func (r *pgxStore) GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Deposit, error) {
	row := tx.QueryRow(ctx, `SELECT `+cols+` FROM deposits WHERE id = $1 FOR UPDATE`, id)
	d, err := scan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("depositStore.GetByIDTx: %w", err)
	}
	return d, nil
}

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Deposit, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+cols+` FROM deposits WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("depositStore.ListByUser: %w", err)
	}
	defer rows.Close()
	var out []*Deposit
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("depositStore.ListByUser scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgxStore) UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status Status, confirmedAt *time.Time) error {
	tag, err := tx.Exec(ctx,
		`UPDATE deposits SET status = $1, confirmed_at = $2 WHERE id = $3`,
		status, confirmedAt, id,
	)
	if err != nil {
		return fmt.Errorf("depositStore.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}
