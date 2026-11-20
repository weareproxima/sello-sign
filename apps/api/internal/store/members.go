package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/model"
)

// ErrAccountBlocked: the account can't be deleted while it's the only
// owner of a space with other members, or the only member of a space whose
// organizations other spaces use (AccountSpaces says which).
var ErrAccountBlocked = errors.New("store: account deletion blocked")

// RemoveMember takes a person added in Sign (not a seat billing pushed) out
// of a space, as someone with role actor; self is a person leaving by
// themselves (model.CanRemove). The space never loses its last owner:
// ErrLastOwner. What the space has (its keys, its log, the invitations the
// person sent) stays: it's the space's.
func (s *Store) RemoveMember(ctx context.Context, workspaceID, userID string, actor model.Role, self bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		managed, _, err := lockWorkspace(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		if managed {
			return ErrMembersManaged
		}
		var role string
		if err := tx.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`,
			workspaceID, userID).Scan(&role); err != nil {
			return notFound(err)
		}
		if !model.CanRemove(actor, model.Role(role), self) {
			return ErrRoleForbidden
		}
		if model.Role(role) == model.Owner {
			var owners int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'owner'`,
				workspaceID).Scan(&owners); err != nil {
				return err
			}
			if owners <= 1 {
				return ErrLastOwner
			}
		}
		label, err := memberLabel(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID); err != nil {
			return err
		}
		e := event{workspace: workspaceID, action: audit.MemberRemoved, target: audit.TargetMember, targetID: userID,
			label: label, details: map[string]any{"role": role}}
		if self {
			e.action = audit.MemberLeft
		}
		return s.record(ctx, tx, e)
	})
}

// AccountOutcome is what deleting a person's account does to one of their
// spaces.
type AccountOutcome string

const (
	// OutcomeLeave: the person leaves it; it keeps its owners.
	OutcomeLeave AccountOutcome = "leave"
	// OutcomeDelete: the person is its only member, and it's deleted with
	// everything in it.
	OutcomeDelete AccountOutcome = "delete"
	// OutcomeTransfer: the person is its only owner and there are other
	// members: someone else has to be made owner first. It blocks.
	OutcomeTransfer AccountOutcome = "transfer"
	// OutcomeDelegated: the person is its only member, but other spaces
	// use its organizations by delegation: those delegations go first. It
	// blocks.
	OutcomeDelegated AccountOutcome = "delegated"
)

// Blocks says whether the outcome stops the account's deletion.
func (o AccountOutcome) Blocks() bool { return o == OutcomeTransfer || o == OutcomeDelegated }

// AccountSpace is one of a person's spaces and what deleting their account
// does to it.
type AccountSpace struct {
	WorkspaceID   string
	Name          string
	Role          model.Role
	Members       int
	Organizations int
	Outcome       AccountOutcome
}

// accountSpaces reads, for each space the person was added to in Sign,
// what deleting their account would do to it; with forUpdate, under a lock
// of those spaces' rows, in id order.
func accountSpaces(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, userID string, forUpdate bool) ([]AccountSpace, error) {
	if forUpdate {
		rows, err := q.Query(ctx, `SELECT w.id FROM workspaces w
			WHERE w.id IN (SELECT workspace_id FROM workspace_members WHERE user_id = $1)
			ORDER BY w.id FOR UPDATE`, userID)
		if err != nil {
			return nil, err
		}
		if _, err := pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return nil, err
		}
	}
	rows, err := q.Query(ctx, `SELECT w.id, w.name, m.role,
			(SELECT count(*) FROM workspace_members x WHERE x.workspace_id = w.id),
			(SELECT count(*) FROM workspace_members x WHERE x.workspace_id = w.id AND x.role = 'owner'),
			(SELECT count(*) FROM organizations o WHERE o.workspace_id = w.id),
			EXISTS (SELECT 1 FROM delegations d JOIN organizations o ON o.id = d.organization_id
				WHERE o.workspace_id = w.id AND d.workspace_id <> w.id)
		FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 ORDER BY w.id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AccountSpace, error) {
		var a AccountSpace
		var role string
		var owners int
		var delegatedOut bool
		if err := r.Scan(&a.WorkspaceID, &a.Name, &role, &a.Members, &owners, &a.Organizations, &delegatedOut); err != nil {
			return a, err
		}
		a.Role = model.Role(role)
		switch {
		case a.Members == 1 && delegatedOut:
			a.Outcome = OutcomeDelegated
		case a.Members == 1:
			a.Outcome = OutcomeDelete
		case a.Role == model.Owner && owners == 1:
			a.Outcome = OutcomeTransfer
		default:
			a.Outcome = OutcomeLeave
		}
		return a, nil
	})
}

// AccountSpaces is what deleting a person's account would do to each of
// the spaces they were added to in Sign, oldest first.
func (s *Store) AccountSpaces(ctx context.Context, userID string) ([]AccountSpace, error) {
	return accountSpaces(ctx, s.pool, userID, false)
}

// DeletedAccount is what deleting an account removed that lives outside
// the database too (DeletedSpaces), and what happened to each space.
type DeletedAccount struct {
	DeletedSpaces
	Spaces []AccountSpace
}

// DeleteAccount deletes what the API keeps of a person: they leave every
// space, the spaces they were the only member of are deleted with
// everything in them (deleteSpaces), and their row in users goes. If a
// space blocks it (AccountOutcome.Blocks), nothing changes:
// ErrAccountBlocked. What they did in the spaces that stay (the keys they
// created, the invitations they sent, their signatures in the log) stays:
// it's those spaces'.
func (s *Store) DeleteAccount(ctx context.Context, userID string) (DeletedAccount, error) {
	var out DeletedAccount
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		spaces, err := accountSpaces(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		out = DeletedAccount{Spaces: spaces}
		var doomed []string
		for _, a := range spaces {
			if a.Outcome.Blocks() {
				return ErrAccountBlocked
			}
			if a.Outcome == OutcomeDelete {
				doomed = append(doomed, a.WorkspaceID)
			}
		}
		if out.DeletedSpaces, err = deleteSpaces(ctx, tx, doomed); err != nil {
			return err
		}
		label, err := memberLabel(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, a := range spaces {
			if a.Outcome != OutcomeLeave {
				continue
			}
			if err := s.record(ctx, tx, event{workspace: a.WorkspaceID, action: audit.MemberLeft, target: audit.TargetMember,
				targetID: userID, label: label, details: map[string]any{"role": a.Role, "account_deleted": true}}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		return err
	})
	if err != nil {
		return DeletedAccount{}, err
	}
	return out, nil
}
