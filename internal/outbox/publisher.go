package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler func(ctx context.Context, event *OutboxEvent) error

type Publisher struct {
	db       *pgxpool.Pool
	store    Store
	handler  Handler
	interval time.Duration
	batch    int
}

func NewPublisher(
	db *pgxpool.Pool,
	store Store,
	handler Handler,
	interval time.Duration,
) *Publisher {
	if interval <= 0 {
		interval = time.Second
	}
	if handler == nil {
		handler = noopHandler
	}
	return &Publisher{
		db:       db,
		store:    store,
		handler:  handler,
		interval: interval,
		batch:    50,
	}
}

func noopHandler(_ context.Context, event *OutboxEvent) error {
	slog.Info("outbox event published", "id", event.ID, "type", event.EventType)
	return nil
}

func (p *Publisher) Start(ctx context.Context) {
	slog.Info("outbox publisher started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox publisher stopped")
			return
		case <-ticker.C:
			if err := p.processBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("outbox processBatch failed", "err", err)
			}
		}
	}
}

func (p *Publisher) processBatch(ctx context.Context) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	events, err := p.store.ListPending(ctx, tx, p.batch)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return tx.Commit(ctx)
	}

	for i := range events {
		event := &events[i]
		if err := p.handler(ctx, event); err != nil {
			slog.Warn("outbox handler failed", "id", event.ID, "err", err)
			if err := p.store.IncrementRetries(ctx, tx, event.ID); err != nil {
				return err
			}
			continue
		}
		if err := p.store.MarkProcessed(ctx, tx, event.ID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
