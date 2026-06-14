// Package asset is the read-only catalog of supported assets and the
// networks each can be deposited or withdrawn on.
//
// Asset and Network rows are seeded by migration and treated as immutable
// configuration at runtime (the application never writes here). Other
// packages depend on this catalog to:
//   - auto-provision a user's per-asset balance row on signup,
//   - validate that a deposit/withdrawal network is supported for the asset,
//   - render the FE's supported-assets list.
package asset

import (
	"time"

	"github.com/google/uuid"
)

type Asset struct {
	Symbol        string     `db:"symbol"`
	Name          string     `db:"name"`
	Decimals      int        `db:"decimals"`
	MinDeposit    string     `db:"min_deposit"`
	MinWithdrawal string     `db:"min_withdrawal"`
	Enabled       bool       `db:"enabled"`
	CreatedAt     *time.Time `db:"created_at"`
}

type Network struct {
	ID               uuid.UUID  `db:"id"`
	AssetSymbol      string     `db:"asset_symbol"`
	Name             string     `db:"name"`
	ChainID          string     `db:"chain_id"`
	AddressPattern   string     `db:"address_pattern"`
	WithdrawalFee    string     `db:"withdrawal_fee"`
	MinConfirmations int        `db:"min_confirmations"`
	Enabled          bool       `db:"enabled"`
	CreatedAt        *time.Time `db:"created_at"`
}
