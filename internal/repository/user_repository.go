package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/domain"
	"mynance/internal/service"
)

type pgxUserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(
	db *pgxpool.Pool,
) service.UserStore {
	return &pgxUserRepository{
		db: db,
	}
}

func (r *pgxUserRepository) Create(ctx context.Context, tx pgx.Tx, user *domain.User) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO users (id, email, username, full_name, password_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		user.ID, user.Email, user.Username, user.FullName, user.PasswordHash, user.Status, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return fmt.Errorf("userRepository.Create: %w", err)
	}
	return nil
}

func (r *pgxUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user := &domain.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, username, full_name, password_hash, status, deleted_at, created_at, updated_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&user.ID, &user.Email, &user.Username, &user.FullName, &user.PasswordHash, &user.Status, &user.DeletedAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("userRepository.GetByID: %w", err)
	}
	return user, nil
}

func (r *pgxUserRepository) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, email, username, full_name, password_hash, status, deleted_at, created_at, updated_at
		 FROM users WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("userRepository.List: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		if err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.FullName, &user.PasswordHash, &user.Status, &user.DeletedAt, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("userRepository.List scan: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *pgxUserRepository) Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		"UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("userRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
