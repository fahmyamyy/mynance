package ledger_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mynance/internal/ledger"
)

type stubLedgerStore struct {
	balance string
	err     error
}

func (s *stubLedgerStore) Insert(_ context.Context, _ pgx.Tx, _ *ledger.LedgerEntry) error {
	return nil
}

func (s *stubLedgerStore) SumByUserAsset(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return s.balance, s.err
}

func (s *stubLedgerStore) ListByUser(_ context.Context, _ ledger.ListFilter) ([]*ledger.LedgerEntry, error) {
	return nil, nil
}

func (s *stubLedgerStore) CountByUser(_ context.Context, _ ledger.ListFilter) (int, error) {
	return 0, nil
}

func TestLedgerService_SumByUserAsset_ZeroWhenNoEntries(t *testing.T) {
	svc := ledger.NewService(&stubLedgerStore{balance: "0"})

	balance, err := svc.SumByUserAsset(context.Background(), uuid.New(), "BTC")

	require.NoError(t, err)
	assert.Equal(t, "0", balance)
}

func TestLedgerService_SumByUserAsset_CorrectSum(t *testing.T) {
	svc := ledger.NewService(&stubLedgerStore{balance: "1.5000000000"})

	balance, err := svc.SumByUserAsset(context.Background(), uuid.New(), "BTC")

	require.NoError(t, err)
	assert.Equal(t, "1.5000000000", balance)
}
