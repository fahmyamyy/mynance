package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	List(ctx context.Context, limit, offset int) ([]*User, error)
	Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
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

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, user *User) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO users (id, email, username, full_name, password_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		user.ID, user.Email, user.Username, user.FullName, user.PasswordHash, user.Status, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return shared.ErrConflict
		}
		return fmt.Errorf("userStore.Create: %w", err)
	}
	return nil
}

func (r *pgxStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, username, full_name, password_hash, status, deleted_at, created_at, updated_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.PasswordHash, &u.Status, &u.DeletedAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("userStore.GetByID: %w", err)
	}
	return u, nil
}

func (r *pgxStore) List(ctx context.Context, limit, offset int) ([]*User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, email, username, full_name, password_hash, status, deleted_at, created_at, updated_at
		 FROM users WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("userStore.List: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.PasswordHash, &u.Status, &u.DeletedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("userStore.List scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *pgxStore) Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		"UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("userStore.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}
