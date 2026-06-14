package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mynance/internal/shared"
	"mynance/pkg/timeutil"
)

// AssetCatalog is the subset of internal/asset.Service this package needs to
// know about. Declared on the consumer side per the project convention.
type AssetCatalog interface {
	EnabledSymbols(ctx context.Context) ([]string, error)
}

// Provisioner inserts a balance row for every enabled asset when a new user
// signs up. It satisfies user.AssetProvisioner — the dependency goes one
// direction (user → account) so user does not need to import account.
type Provisioner struct {
	store   Store
	catalog AssetCatalog
}

func NewProvisioner(store Store, catalog AssetCatalog) *Provisioner {
	return &Provisioner{store: store, catalog: catalog}
}

// ProvisionAll inserts an account row for every enabled asset, inside the
// caller's transaction. Idempotent w.r.t. the (user_id, asset) unique index
// — pre-existing rows are silently skipped via shared.ErrConflict.
func (p *Provisioner) ProvisionAll(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	symbols, err := p.catalog.EnabledSymbols(ctx)
	if err != nil {
		return fmt.Errorf("Provisioner: list assets: %w", err)
	}
	now := timeutil.Now()
	for _, symbol := range symbols {
		id, err := NewAccountID()
		if err != nil {
			return fmt.Errorf("Provisioner: id: %w", err)
		}
		acct := &Account{
			ID:        id,
			UserID:    userID,
			Asset:     symbol,
			CreatedAt: &now,
		}
		if err := p.store.Create(ctx, tx, acct); err != nil {
			if errors.Is(err, shared.ErrConflict) {
				continue
			}
			return fmt.Errorf("Provisioner: insert %s: %w", symbol, err)
		}
	}
	return nil
}
