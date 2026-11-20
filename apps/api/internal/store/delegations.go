package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
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

// CreateDelegation lets workspaceID use the organization with scopes, as
// its owner.
func (s *Store) CreateDelegation(ctx context.Context, organizationID, workspaceID string, scopes []model.Scope, locked bool) (Delegation, error) {
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var owner, name string
		if err := tx.QueryRow(ctx, `SELECT workspace_id, name FROM organizations WHERE id = $1`, organizationID).
			Scan(&owner, &name); err != nil {
			return notFound(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO delegations (id, organization_id, workspace_id, scopes, locked)
			VALUES ($1, $2, $3, $4, $5)`, id, organizationID, workspaceID, model.Strings(scopes), locked)
		if _, ok := uniqueViolation(err); ok {
			return ErrDelegationExists
		}
		if err != nil {
			return err
		}
		return s.recordDelegation(ctx, tx, audit.DelegationGranted, owner, owner, organizationID, name, delegationFacts{
			id: id, delegate: workspaceID, scopes: model.Strings(scopes), locked: locked,
		})
	})
	if err != nil {
		return Delegation{}, err
	}
	return s.Delegation(ctx, organizationID, id)
}

// DeleteDelegation removes a delegation, as its owner or, with force, as
// the delegate renouncing it. Unless force, a locked one is
// ErrDelegationLocked.
func (s *Store) DeleteDelegation(ctx context.Context, organizationID, id string, force bool) error {
	if !ids.Valid(id) {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var d delegationFacts
		var owner, name string
		if err := tx.QueryRow(ctx, `SELECT d.workspace_id, d.scopes, d.locked, o.workspace_id, o.name
			FROM delegations d JOIN organizations o ON o.id = d.organization_id
			WHERE d.organization_id = $1 AND d.id = $2 FOR UPDATE OF d`,
			organizationID, id).Scan(&d.delegate, &d.scopes, &d.locked, &owner, &name); err != nil {
			return notFound(err)
		}
		if d.locked && !force {
			return ErrDelegationLocked
		}
		if _, err := tx.Exec(ctx, `DELETE FROM delegations WHERE id = $1`, id); err != nil {
			return err
		}
		d.id = id
		acting := owner
		if force {
			acting, d.renounced = d.delegate, true
		}
		return s.recordDelegation(ctx, tx, audit.DelegationRemoved, acting, owner, organizationID, name, d)
	})
}

// delegationFacts are what an event of a delegation says of it.
type delegationFacts struct {
	id       string
	delegate string
	scopes   []string
	locked   bool
	// renounced: the delegate gave it up.
	renounced bool
}

// recordDelegation records action on a delegation of an organization
// (organizationID, named name, owned by owner) in the space acting, with
// the context's actor, and in the delegation's other space, with acting as
// the actor.
func (s *Store) recordDelegation(ctx context.Context, tx pgx.Tx, action audit.Action, acting, owner, organizationID, name string, d delegationFacts) error {
	ownerName, err := workspaceName(ctx, tx, owner)
	if err != nil {
		return err
	}
	delegateName, err := workspaceName(ctx, tx, d.delegate)
	if err != nil {
		return err
	}
	details := map[string]any{
		"organization_id": organizationID, "owner_workspace_id": owner, "owner_workspace_name": ownerName,
		"workspace_id": d.delegate, "workspace_name": delegateName, "scopes": d.scopes, "locked": d.locked,
	}
	if action == audit.DelegationRemoved {
		details["renounced"] = d.renounced
	}
	e := event{workspace: acting, action: action, target: audit.TargetDelegation, targetID: d.id, label: name, details: details}
	if err := s.record(ctx, tx, e); err != nil {
		return err
	}
	other := d.delegate
	if acting == d.delegate {
		other = owner
	}
	if other == acting {
		return nil
	}
	by := audit.Space(acting)
	e.workspace, e.actor = other, &by
	return s.record(ctx, tx, e)
}
