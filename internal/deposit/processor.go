package deposit

import (
	"context"

	"github.com/google/uuid"

	"mynance/internal/shared"
)

// Processor decides what happens when a user calls POST /deposits. The real
// implementation rejects the call (user-initiated deposits do not exist —
// real funds arrive via the on-chain listener and admin intake). The sandbox
// implementation fakes an on-chain settlement so the FE can exercise the
// ledger end-to-end without leaving the simulator.
type Processor interface {
	Deposit(ctx context.Context, userID uuid.UUID, cmd IntakeCommand) (*Deposit, error)
}

type realProcessor struct{}

// NewProcessor wires the production deposit endpoint. The endpoint
// stays mounted for a stable FE contract, but every call returns 501 —
// real deposits never originate from a user request.
func NewProcessor() Processor {
	return &realProcessor{}
}

func (p *realProcessor) Deposit(context.Context, uuid.UUID, IntakeCommand) (*Deposit, error) {
	return nil, shared.ErrNotImplemented
}

type sandboxProcessor struct {
	svc Service
}

// NewSandboxProcessor wires the sandbox deposit endpoint, delegating to the
// existing Simulate flow on the deposit service so the ledger/outbox path is
// identical to a real intake+confirm pair.
func NewSandboxProcessor(svc Service) Processor {
	return &sandboxProcessor{svc: svc}
}

func (p *sandboxProcessor) Deposit(ctx context.Context, userID uuid.UUID, cmd IntakeCommand) (*Deposit, error) {
	return p.svc.Simulate(ctx, userID, cmd)
}
