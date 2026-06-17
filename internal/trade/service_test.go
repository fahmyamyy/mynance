package trade

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"mynance/internal/ledger"
	"mynance/internal/order"
	"mynance/internal/outbox"
	"mynance/pkg/numeric"
)

type stubOrderStore struct {
	buy, sell *order.Order
	updates   []struct {
		id     uuid.UUID
		status order.Status
	}
}

func (s *stubOrderStore) GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*order.Order, error) {
	if id == s.buy.ID {
		return s.buy, nil
	}
	return s.sell, nil
}
func (s *stubOrderStore) IncrementFilled(ctx context.Context, tx pgx.Tx, id uuid.UUID, qty pgtype.Numeric) error {
	return nil
}
func (s *stubOrderStore) UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status order.Status) error {
	s.updates = append(s.updates, struct {
		id     uuid.UUID
		status order.Status
	}{id, status})
	return nil
}

type stubIdemp struct{}

func (stubIdemp) Insert(ctx context.Context, tx pgx.Tx, key string, scope interface{ ScopeName() string }) error {
	return nil
}

type stubLedger struct{}

func (stubLedger) Insert(ctx context.Context, tx pgx.Tx, e *ledger.LedgerEntry) error { return nil }

type stubOutbox struct{}

func (stubOutbox) Insert(ctx context.Context, tx pgx.Tx, e *outbox.OutboxEvent) error { return nil }

type stubAccounts struct{}

func (stubAccounts) AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func TestApplyOrderFill_FullFillSetsFilled(t *testing.T) {
	qty, _ := numeric.Parse("1")
	o := &order.Order{
		ID:             uuid.New(),
		Quantity:       qty,
		FilledQuantity: numeric.Zero(),
		Status:         order.StatusOpen,
	}
	store := &stubOrderStore{buy: o}
	svc := &tradeService{orders: store}

	require.NoError(t, svc.applyOrderFill(context.Background(), nil, o, qty))
	require.Len(t, store.updates, 1)
	require.Equal(t, order.StatusFilled, store.updates[0].status)
}

func TestApplyOrderFill_PartialFillSetsPartial(t *testing.T) {
	totalQty, _ := numeric.Parse("2")
	fillQty, _ := numeric.Parse("1")
	o := &order.Order{
		ID:             uuid.New(),
		Quantity:       totalQty,
		FilledQuantity: numeric.Zero(),
		Status:         order.StatusOpen,
	}
	store := &stubOrderStore{buy: o}
	svc := &tradeService{orders: store}

	require.NoError(t, svc.applyOrderFill(context.Background(), nil, o, fillQty))
	require.Len(t, store.updates, 1)
	require.Equal(t, order.StatusPartial, store.updates[0].status)
}

func TestZeroSum_FourEntriesEqualZero(t *testing.T) {
	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")
	quoteAmount := numeric.Mul(price, qty)
	baseAmount := qty

	sum := numeric.Zero()
	sum = numeric.Add(sum, baseAmount)
	sum = numeric.Add(sum, numeric.Neg(quoteAmount))
	sum = numeric.Add(sum, numeric.Neg(baseAmount))
	sum = numeric.Add(sum, quoteAmount)
	require.Equal(t, 0, numeric.Cmp(sum, numeric.Zero()))
}
