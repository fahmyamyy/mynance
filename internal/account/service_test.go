package account_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mynance/internal/account"
)

type stubAccountStore struct{}

func (s *stubAccountStore) Create(_ context.Context, _ pgx.Tx, _ *account.Account) error {
	return nil
}
func (s *stubAccountStore) GetByID(_ context.Context, _ uuid.UUID) (*account.Account, error) {
	return nil, nil
}
func (s *stubAccountStore) GetByUserAndAsset(_ context.Context, _ uuid.UUID, _ string) (*account.Account, error) {
	return nil, nil
}
func (s *stubAccountStore) List(_ context.Context, _, _ int) ([]*account.Account, error) {
	return nil, nil
}
func (s *stubAccountStore) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]*account.Account, error) {
	return nil, nil
}
func (s *stubAccountStore) Delete(_ context.Context, _ pgx.Tx, _ uuid.UUID) error {
	return nil
}

type stubLedgerClient struct {
	balance string
	err     error
}

func (s *stubLedgerClient) SumByUserAsset(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return s.balance, s.err
}

func TestAccountService_GetBalance_ZeroWhenNoEntries(t *testing.T) {
	svc := account.NewService(nil, &stubAccountStore{}, &stubLedgerClient{balance: "0"})

	balance, err := svc.GetBalance(context.Background(), uuid.New(), "BTC")

	require.NoError(t, err)
	assert.Equal(t, "0", balance)
}
