package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
	"mynance/pkg/crypto"
	"mynance/pkg/timeutil"
)

type CreateUserCommand struct {
	Email    string
	Username string
	FullName string
	Password string
}

type Service interface {
	CreateUser(ctx context.Context, cmd CreateUserCommand) (*User, error)
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)
	ListUsers(ctx context.Context, limit, offset int) ([]*User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
	Login(ctx context.Context, email, password string) (*User, error)
}

// AssetProvisioner inserts a balance row for every supported asset when a
// new user signs up. Implementations are expected to be idempotent — at
// minimum they must not error on (user_id, asset) rows that already exist.
type AssetProvisioner interface {
	ProvisionAll(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error
}

// noopProvisioner is used when the caller wires the user service without
// asset provisioning (e.g. tests that only exercise auth).
type noopProvisioner struct{}

func (noopProvisioner) ProvisionAll(context.Context, pgx.Tx, uuid.UUID) error { return nil }

type userService struct {
	db          *pgxpool.Pool
	store       Store
	provisioner AssetProvisioner
}

func NewService(
	db *pgxpool.Pool,
	store Store,
) Service {
	return &userService{
		db:          db,
		store:       store,
		provisioner: noopProvisioner{},
	}
}

// NewServiceWithProvisioner wires the user service so CreateUser also seeds
// per-asset balance rows inside the same transaction.
func NewServiceWithProvisioner(
	db *pgxpool.Pool,
	store Store,
	provisioner AssetProvisioner,
) Service {
	if provisioner == nil {
		provisioner = noopProvisioner{}
	}
	return &userService{
		db:          db,
		store:       store,
		provisioner: provisioner,
	}
}

func (s *userService) CreateUser(ctx context.Context, cmd CreateUserCommand) (*User, error) {
	id, err := NewUserID()
	if err != nil {
		return nil, fmt.Errorf("CreateUser generate id: %w", err)
	}

	passwordHash, err := crypto.HashPassword(cmd.Password)
	if err != nil {
		return nil, fmt.Errorf("CreateUser hash password: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateUser begin: %w", err)
	}
	defer tx.Rollback(ctx)

	now := timeutil.Now()
	u := &User{
		ID:           id,
		Email:        cmd.Email,
		Username:     cmd.Username,
		FullName:     cmd.FullName,
		PasswordHash: passwordHash,
		Status:       "ACTIVE",
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	if err := s.store.Create(ctx, tx, u); err != nil {
		return nil, fmt.Errorf("CreateUser: %w", err)
	}
	if err := s.provisioner.ProvisionAll(ctx, tx, id); err != nil {
		return nil, fmt.Errorf("CreateUser provision assets: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateUser commit: %w", err)
	}
	return u, nil
}

func (s *userService) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetUser: %w", err)
	}
	return u, nil
}

func (s *userService) ListUsers(ctx context.Context, limit, offset int) ([]*User, error) {
	users, err := s.store.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ListUsers: %w", err)
	}
	return users, nil
}

func (s *userService) Login(ctx context.Context, email, password string) (*User, error) {
	u, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrUnauthorized
		}
		return nil, fmt.Errorf("Login lookup: %w", err)
	}
	if !crypto.CheckPassword(password, u.PasswordHash) {
		return nil, shared.ErrUnauthorized
	}
	return u, nil
}

func (s *userService) DeleteUser(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("DeleteUser begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.store.Delete(ctx, tx, id); err != nil {
		return fmt.Errorf("DeleteUser: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteUser commit: %w", err)
	}
	return nil
}
