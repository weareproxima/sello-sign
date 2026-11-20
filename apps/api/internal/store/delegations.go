package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

const delegationColumns = `d.id, d.organization_id, d.workspace_id, w.name, d.scopes, d.locked, d.created_at`

func scanDelegation(row pgx.Row) (Delegation, error) {
	var d Delegation
	var scopes []string
	if err := row.Scan(&d.ID, &d.OrganizationID, &d.WorkspaceID, &d.WorkspaceName, &scopes, &d.Locked, &d.CreatedAt); err != nil {
		return Delegation{}, notFound(err)
	}
	d.Scopes = model.ScopesOf(scopes)
	return d, nil
}

// Delegations lists an organization's delegations, oldest first.
func (s *Store) Delegations(ctx context.Context, organizationID string) ([]Delegation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+delegationColumns+` FROM delegations d
		JOIN workspaces w ON w.id = d.workspace_id WHERE d.organization_id = $1 ORDER BY d.id`, organizationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Delegation, error) { return scanDelegation(r) })
}

// Delegation reads one of an organization's delegations.
func (s *Store) Delegation(ctx context.Context, organizationID, id string) (Delegation, error) {
	if !ids.Valid(id) {
		return Delegation{}, ErrNotFound
	}
	return scanDelegation(s.pool.QueryRow(ctx, `SELECT `+delegationColumns+` FROM delegations d
		JOIN workspaces w ON w.id = d.workspace_id WHERE d.organization_id = $1 AND d.id = $2`, organizationID, id))
}

// CreateDelegation lets workspaceID use the organization with scopes.
func (s *Store) CreateDelegation(ctx context.Context, organizationID, workspaceID string, scopes []model.Scope, locked bool) (Delegation, error) {
	id := ids.New()
	_, err := s.pool.Exec(ctx, `INSERT INTO delegations (id, organization_id, workspace_id, scopes, locked)
		VALUES ($1, $2, $3, $4, $5)`, id, organizationID, workspaceID, model.Strings(scopes), locked)
	if _, ok := uniqueViolation(err); ok {
		return Delegation{}, ErrDelegationExists
	}
	if err != nil {
		return Delegation{}, err
	}
	return s.Delegation(ctx, organizationID, id)
}

// DeleteDelegation removes a delegation. Unless force (the delegate
// renouncing it), a locked one is ErrDelegationLocked.
func (s *Store) DeleteDelegation(ctx context.Context, organizationID, id string, force bool) error {
	if !ids.Valid(id) {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT locked FROM delegations WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
			organizationID, id).Scan(&locked); err != nil {
			return notFound(err)
		}
		if locked && !force {
			return ErrDelegationLocked
		}
		_, err := tx.Exec(ctx, `DELETE FROM delegations WHERE id = $1`, id)
		return err
	})
}
