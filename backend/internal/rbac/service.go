package rbac

import (
	"context"

	"github.com/google/uuid"

	"github.com/qeetgroup/qeet-identity/internal/audit"
)

// Service wraps the Repository so each mutation owns its transaction and writes
// the audit row in the same tx — keeping the audit trail atomic with the change
// and the HTTP handlers thin. Reads stay on the Repository.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateRole(ctx context.Context, tenantID uuid.UUID, name, desc string, actor audit.Actor) (*Role, error) {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	role, err := s.repo.CreateRole(ctx, tx, tenantID, name, desc, false)
	if err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, actor.Event(tenantID, "role.created", "role", role.ID,
		map[string]any{"name": role.Name, "is_system": role.IsSystem})); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *Service) GrantPermission(ctx context.Context, roleID, permID uuid.UUID, actor audit.Actor) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.repo.GrantPermission(ctx, tx, roleID, permID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor.PlatformEvent("role.permission_granted", "role", roleID,
		map[string]any{"permission_id": permID})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RevokePermission(ctx context.Context, roleID, permID uuid.UUID, actor audit.Actor) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.repo.RevokePermission(ctx, tx, roleID, permID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor.PlatformEvent("role.permission_revoked", "role", roleID,
		map[string]any{"permission_id": permID})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AssignRole(ctx context.Context, userID, tenantID, roleID uuid.UUID, grantedBy *uuid.UUID, actor audit.Actor) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.repo.AssignRole(ctx, tx, userID, tenantID, roleID, grantedBy); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor.Event(tenantID, "role.assigned", "user", userID,
		map[string]any{"role_id": roleID})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) UnassignRole(ctx context.Context, userID, tenantID, roleID uuid.UUID, actor audit.Actor) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.repo.UnassignRole(ctx, tx, userID, tenantID, roleID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor.Event(tenantID, "role.unassigned", "user", userID,
		map[string]any{"role_id": roleID})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
