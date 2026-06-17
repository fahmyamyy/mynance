package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ListFilter scopes a ledger list query. UserID is mandatory — even the
// admin endpoint pins a single user per request so the result set stays
// bounded. The slice/pointer fields are zero-value-skip: empty Asset and
// nil From/To mean "no constraint".
type ListFilter struct {
	UserID     uuid.UUID
	Asset      string
	EntryTypes []EntryType
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
	ListByUser(ctx context.Context, filter ListFilter) ([]*LedgerEntry, error)
	CountByUser(ctx context.Context, filter ListFilter) (int, error)
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

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO ledger_entries (id, user_id, account_id, asset, amount, entry_type, ref_type, ref_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ID, entry.UserID, entry.AccountID, entry.Asset,
		entry.Amount, entry.EntryType, entry.RefType, entry.RefID, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("ledgerStore.Insert: %w", err)
	}
	return nil
}

// buildWhere assembles the dynamic WHERE clause shared by ListByUser and
// CountByUser. Returns the SQL fragment, the positional args, and the next
// placeholder index so callers can append LIMIT/OFFSET (or anything else).
func buildWhere(filter ListFilter) (clause string, args []any, nextIdx int) {
	conds := []string{"user_id = $1"}
	args = []any{filter.UserID}
	idx := 2
	if filter.Asset != "" {
		conds = append(conds, fmt.Sprintf("asset = $%d", idx))
		args = append(args, filter.Asset)
		idx++
	}
	if len(filter.EntryTypes) > 0 {
		types := make([]string, len(filter.EntryTypes))
		for i, t := range filter.EntryTypes {
			types[i] = string(t)
		}
		conds = append(conds, fmt.Sprintf("entry_type = ANY($%d)", idx))
		args = append(args, types)
		idx++
	}
	if filter.From != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.From)
		idx++
	}
	if filter.To != nil {
		conds = append(conds, fmt.Sprintf("created_at < $%d", idx))
		args = append(args, *filter.To)
		idx++
	}
	return strings.Join(conds, " AND "), args, idx
}

func (r *pgxStore) ListByUser(ctx context.Context, filter ListFilter) ([]*LedgerEntry, error) {
	where, args, idx := buildWhere(filter)
	query := `SELECT id, user_id, account_id, asset, amount, entry_type, ref_type, ref_id, created_at
		FROM ledger_entries
		WHERE ` + where + fmt.Sprintf(`
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ledgerStore.ListByUser: %w", err)
	}
	defer rows.Close()

	var out []*LedgerEntry
	for rows.Next() {
		e := &LedgerEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.AccountID, &e.Asset, &e.Amount,
			&e.EntryType, &e.RefType, &e.RefID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("ledgerStore.ListByUser scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *pgxStore) CountByUser(ctx context.Context, filter ListFilter) (int, error) {
	where, args, _ := buildWhere(filter)
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE `+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("ledgerStore.CountByUser: %w", err)
	}
	return total, nil
}

func (r *pgxStore) SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	var sum pgtype.Numeric
	err := r.db.QueryRow(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE user_id = $1 AND asset = $2",
		userID, asset,
	).Scan(&sum)
	if err != nil {
		return "", fmt.Errorf("ledgerStore.SumByUserAsset: %w", err)
	}

	if !sum.Valid {
		return "0", nil
	}

	text, err := sum.Value()
	if err != nil {
		return "", fmt.Errorf("ledgerStore.SumByUserAsset numeric value: %w", err)
	}
	if text == nil {
		return "0", nil
	}
	return fmt.Sprintf("%v", text), nil
}
