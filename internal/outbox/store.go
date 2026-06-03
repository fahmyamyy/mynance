package outbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/pkg/timeutil"
)

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, event *OutboxEvent) error
	ListPending(ctx context.Context, tx pgx.Tx, limit int) ([]OutboxEvent, error)
	MarkProcessed(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
	IncrementRetries(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
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

func (r *pgxStore) Pool() *pgxpool.Pool {
	return r.db
}

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, event *OutboxEvent) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (id, event_type, payload, status, retries, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		event.ID, event.EventType, event.Payload, StatusPending, 0, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("outboxStore.Insert: %w", err)
	}
	return nil
}

func (r *pgxStore) ListPending(ctx context.Context, tx pgx.Tx, limit int) ([]OutboxEvent, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, event_type, payload, status, retries, processed_at, created_at
		 FROM outbox_events
		 WHERE status = 'PENDING'
		 ORDER BY created_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outboxStore.ListPending query: %w", err)
	}
	defer rows.Close()

	var out []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.Status, &e.Retries, &e.ProcessedAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("outboxStore.ListPending scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outboxStore.ListPending rows: %w", err)
	}
	return out, nil
}

func (r *pgxStore) MarkProcessed(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	now := timeutil.Now()
	_, err := tx.Exec(ctx,
		`UPDATE outbox_events SET status = 'PROCESSED', processed_at = $1 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("outboxStore.MarkProcessed: %w", err)
	}
	return nil
}

func (r *pgxStore) IncrementRetries(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`UPDATE outbox_events SET retries = retries + 1 WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("outboxStore.IncrementRetries: %w", err)
	}
	return nil
}
