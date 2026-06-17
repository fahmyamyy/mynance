package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"mynance/internal/ledger"
	"mynance/internal/outbox"
	"mynance/internal/shared"
)

type stubStore struct {
	order *Order
}

func (s *stubStore) Create(ctx context.Context, tx pgx.Tx, o *Order) error { return nil }
func (s *stubStore) GetByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	return s.order, nil
}
func (s *stubStore) GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Order, error) {
	return s.order, nil
}
func (s *stubStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error) {
	return nil, nil
}
func (s *stubStore) UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status Status) error {
	return nil
}
func (s *stubStore) IncrementFilled(ctx context.Context, tx pgx.Tx, id uuid.UUID, qty pgtype.Numeric) error {
	return nil
}

type stubLedger struct{}

func (stubLedger) Insert(ctx context.Context, tx pgx.Tx, e *ledger.LedgerEntry) error { return nil }
func (stubLedger) SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	return "0", nil
}

type stubOutbox struct{}

func (stubOutbox) Insert(ctx context.Context, tx pgx.Tx, e *outbox.OutboxEvent) error { return nil }

type stubAccounts struct{}

func (stubAccounts) AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func TestCancelOrder_TerminalStatus_ReturnsInvalidStateTransition(t *testing.T) {
	cases := []struct {
		name   string
		status Status
	}{
		{"FILLED", StatusFilled},
		{"CANCELLED", StatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubStore{order: &Order{
				ID:     uuid.New(),
				Symbol: "BTC-USDT",
				Side:   SideBuy,
				Status: tc.status,
			}}
			svc := &orderService{
				store:       store,
				idempotency: nil,
				ledger:      stubLedger{},
				outbox:      stubOutbox{},
				accounts:    stubAccounts{},
			}
			_, err := svc.CancelOrder(context.Background(), uuid.New())
			require.Error(t, err)
			require.True(t, errors.Is(err, shared.ErrInvalidStateTransition))
		})
	}
}

type stubEngine struct {
	placed []string
}

func (e *stubEngine) SubmitPlace(orderID, _, _, _ string, _, _ float64) error {
	e.placed = append(e.placed, orderID)
	return nil
}
func (e *stubEngine) SubmitCancel(_, _ string) error { return nil }

func TestNoopEngine_DefaultWhenNotProvided(t *testing.T) {
	svc := &orderService{engine: noopEngine{}}
	require.NoError(t, svc.engine.SubmitPlace("a", "b", "c", "d", 1, 1))
	require.NoError(t, svc.engine.SubmitCancel("a", "c"))
}

func TestSplitSymbol(t *testing.T) {
	base, quote, err := SplitSymbol("BTC-USDT")
	require.NoError(t, err)
	require.Equal(t, "BTC", base)
	require.Equal(t, "USDT", quote)

	_, _, err = SplitSymbol("BTCUSDT")
	require.Error(t, err)
}
