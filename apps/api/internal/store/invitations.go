package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

// InvitationLifetime is how long an invitation can be accepted.
const InvitationLifetime = 7 * 24 * time.Hour

var (
	// ErrMembersManaged: the space is of an organization of Sello's IdP,
	// whose members are the seats billing pushes; they're invited and
	// changed in Sello's account center.
	ErrMembersManaged = errors.New("store: members managed by Sello")
	// ErrAlreadyMember: the address is of someone already in the space.
	ErrAlreadyMember = errors.New("store: already a member")
	// ErrInvitationExists: the address has an open invitation to the space.
	ErrInvitationExists = errors.New("store: invitation exists")
	// ErrWorkspaceFull: the plan admits no more members.
	ErrWorkspaceFull = errors.New("store: workspace full")
	// ErrInvitationExpired: the invitation can no longer be accepted.
	ErrInvitationExpired = errors.New("store: invitation expired")
	// ErrInvitationUsed: someone else already accepted the invitation.
	ErrInvitationUsed = errors.New("store: invitation used")
	// ErrEmailMismatch: the invitation is for another address.
	ErrEmailMismatch = errors.New("store: invitation for another address")
	// ErrRoleForbidden: the caller's role can't make that change.
	ErrRoleForbidden = errors.New("store: role change forbidden")
	// ErrLastOwner: the change would leave the space without an owner.
	ErrLastOwner = errors.New("store: last owner")
)

// Invitation is an invitation to a space.
type Invitation struct {
	ID            string
	WorkspaceID   string
	WorkspaceName string
	Email         string
	Role          model.Role
	InvitedBy     Member
	ExpiresAt     time.Time
	AcceptedAt    *time.Time
	AcceptedBy    *string
	CreatedAt     time.Time
}

// Expired says whether it can no longer be accepted at now.
func (i Invitation) Expired(now time.Time) bool { return !now.Before(i.ExpiresAt) }

const invitationColumns = `i.id, i.workspace_id, w.name, i.email, i.role, i.invited_by, u.email, u.name,
	i.expires_at, i.accepted_at, i.accepted_by, i.created_at`

const invitationFrom = ` FROM invitations i JOIN workspaces w ON w.id = i.workspace_id
	LEFT JOIN users u ON u.id = i.invited_by`

func scanInvitation(row pgx.Row) (Invitation, error) {
	var i Invitation
	var role, by string
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.WorkspaceName, &i.Email, &role, &by, &i.InvitedBy.Email, &i.InvitedBy.Name,
		&i.ExpiresAt, &i.AcceptedAt, &i.AcceptedBy, &i.CreatedAt)
	if err != nil {
		return Invitation{}, notFound(err)
	}
	i.Role = model.Role(role)
	i.InvitedBy.UserID = &by
	return i, nil
}

// NewInvitation is what creates an invitation; Hash comes from
// auth.NewInvitationToken and Email is normalized.
type NewInvitation struct {
	WorkspaceID string
	Email       string
	Role        model.Role
	Hash        []byte
	InvitedBy   string
}

// lockWorkspace locks a space's row for the transaction, so that changes
// to its members happen one after the other, and says whether billing
// manages them and how many its plan admits.
func lockWorkspace(ctx context.Context, tx pgx.Tx, id string) (managed bool, maxMembers *int, err error) {
	var sello *string
	err = tx.QueryRow(ctx, `SELECT sello_workspace_id, max_members FROM workspaces WHERE id = $1 FOR UPDATE`, id).
		Scan(&sello, &maxMembers)
	return sello != nil, maxMembers, notFound(err)
}

// full says whether the space's own members already fill its plan.
func full(ctx context.Context, tx pgx.Tx, id string, maxMembers *int) (bool, error) {
	if maxMembers == nil {
		return false, nil
	}
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1`, id).Scan(&n)
	return n >= *maxMembers, err
}

// CreateInvitation invites an address to a space for InvitationLifetime.
// An expired invitation to the same address is revoked and replaced; an
// open one is ErrInvitationExists.
func (s *Store) CreateInvitation(ctx context.Context, n NewInvitation) (Invitation, error) {
	id := ids.New()
	now := s.now()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		managed, maxMembers, err := lockWorkspace(ctx, tx, n.WorkspaceID)
		if err != nil {
			return err
		}
		if managed {
			return ErrMembersManaged
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN users u ON u.id = m.user_id
			WHERE m.workspace_id = $1 AND lower(u.email) = $2)`, n.WorkspaceID, n.Email).Scan(&member); err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		if isFull, err := full(ctx, tx, n.WorkspaceID, maxMembers); err != nil || isFull {
			if err == nil {
				err = ErrWorkspaceFull
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE invitations SET revoked_at = $3
			WHERE workspace_id = $1 AND email = $2 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at <= $3`,
			n.WorkspaceID, n.Email, now); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO invitations (id, workspace_id, email, role, hash, invited_by, expires_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, n.WorkspaceID, n.Email, string(n.Role), n.Hash, n.InvitedBy, now.Add(InvitationLifetime), now)
		if c, ok := uniqueViolation(err); ok && c == "invitations_open" {
			return ErrInvitationExists
		}
		if err != nil {
			return err
		}
		return s.record(ctx, tx, event{workspace: n.WorkspaceID, action: audit.InvitationCreated, target: audit.TargetInvitation,
			targetID: id, label: n.Email, details: map[string]any{"role": n.Role}})
	})
	if err != nil {
		return Invitation{}, err
	}
	return scanInvitation(s.pool.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE i.id = $1`, id))
}

// Invitations lists a space's open invitations, expired ones included,
// newest first.
func (s *Store) Invitations(ctx context.Context, workspaceID string) ([]Invitation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+invitationColumns+invitationFrom+`
		WHERE i.workspace_id = $1 AND i.accepted_at IS NULL AND i.revoked_at IS NULL
		ORDER BY i.id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Invitation, error) { return scanInvitation(r) })
}

// RevokeInvitation revokes one of a space's open invitations: its link
// stops working.
func (s *Store) RevokeInvitation(ctx context.Context, workspaceID, id string) error {
	if !ids.Valid(id) {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var email, role string
		if err := tx.QueryRow(ctx, `UPDATE invitations SET revoked_at = $3
			WHERE workspace_id = $1 AND id = $2 AND accepted_at IS NULL AND revoked_at IS NULL RETURNING email, role`,
			workspaceID, id, s.now()).Scan(&email, &role); err != nil {
			return notFound(err)
		}
		return s.record(ctx, tx, event{workspace: workspaceID, action: audit.InvitationRevoked, target: audit.TargetInvitation,
			targetID: id, label: email, details: map[string]any{"role": role}})
	})
}

// InvitationByHash reads the invitation of a token's hash, unless it was
// revoked.
func (s *Store) InvitationByHash(ctx context.Context, hash []byte) (Invitation, error) {
	return scanInvitation(s.pool.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+`
		WHERE i.hash = $1 AND i.revoked_at IS NULL`, hash))
}

// AcceptInvitation makes the person userID, whose address is email, a
// member of the invitation's space with its role, and closes it. Someone
// already in the space keeps their role. Accepting again what the same
// person accepted answers the same.
func (s *Store) AcceptInvitation(ctx context.Context, hash []byte, userID, email string) (Invitation, error) {
	now := s.now()
	var inv Invitation
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		inv, err = scanInvitation(tx.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+`
			WHERE i.hash = $1 AND i.revoked_at IS NULL FOR UPDATE OF i`, hash))
		if err != nil {
			return err
		}
		if inv.AcceptedAt != nil {
			if inv.AcceptedBy != nil && *inv.AcceptedBy == userID {
				return nil
			}
			return ErrInvitationUsed
		}
		if inv.Expired(now) {
			return ErrInvitationExpired
		}
		if model.NormalizeEmail(email) != inv.Email {
			return ErrEmailMismatch
		}
		managed, maxMembers, err := lockWorkspace(ctx, tx, inv.WorkspaceID)
		if err != nil {
			return err
		}
		if managed {
			return ErrMembersManaged
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2)`,
			inv.WorkspaceID, userID).Scan(&member); err != nil {
			return err
		}
		if !member {
			if isFull, err := full(ctx, tx, inv.WorkspaceID, maxMembers); err != nil || isFull {
				if err == nil {
					err = ErrWorkspaceFull
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)`,
				inv.WorkspaceID, userID, string(inv.Role)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE invitations SET accepted_at = $2, accepted_by = $3 WHERE id = $1`, inv.ID, now, userID); err != nil {
			return err
		}
		inv.AcceptedAt, inv.AcceptedBy = &now, &userID
		// joined is false for someone already in the space, who keeps
		// their role.
		return s.record(ctx, tx, event{workspace: inv.WorkspaceID, action: audit.InvitationAccepted, target: audit.TargetInvitation,
			targetID: inv.ID, label: inv.Email, details: map[string]any{"role": inv.Role, "joined": !member}})
	})
	return inv, err
}

// ChangeRole changes the role of a person added in Sign (not a seat billing
// pushed) to to, as someone with role actor (model.CanSetRole). The space
// never loses its last owner: ErrLastOwner.
func (s *Store) ChangeRole(ctx context.Context, workspaceID, userID string, to, actor model.Role) (Member, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		managed, _, err := lockWorkspace(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		if managed {
			return ErrMembersManaged
		}
		var from string
		if err := tx.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`,
			workspaceID, userID).Scan(&from); err != nil {
			return notFound(err)
		}
		if !model.CanSetRole(actor, model.Role(from), to) {
			return ErrRoleForbidden
		}
		if model.Role(from) == to {
			return nil
		}
		if model.Role(from) == model.Owner {
			var owners int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'owner'`,
				workspaceID).Scan(&owners); err != nil {
				return err
			}
			if owners <= 1 {
				return ErrLastOwner
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_members SET role = $3 WHERE workspace_id = $1 AND user_id = $2`,
			workspaceID, userID, string(to)); err != nil {
			return err
		}
		label, err := memberLabel(ctx, tx, userID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, event{workspace: workspaceID, action: audit.MemberRoleChanged, target: audit.TargetMember,
			targetID: userID, label: label, details: map[string]any{"from": from, "to": to}})
	})
	if err != nil {
		return Member{}, err
	}
	var m Member
	var role string
	err = s.pool.QueryRow(ctx, `SELECT m.user_id, u.email, u.name, m.role FROM workspace_members m
		LEFT JOIN users u ON u.id = m.user_id WHERE m.workspace_id = $1 AND m.user_id = $2`, workspaceID, userID).
		Scan(&m.UserID, &m.Email, &m.Name, &role)
	m.Role = model.Role(role)
	return m, notFound(err)
}

// RenewInvitation gives one of a space's open invitations, expired or not,
// a new token (hash) and InvitationLifetime from now, as someone with role
// actor (model.CanInvite for its role): the old link stops working. It's
// the same invitation, with the same id, address and role; the checks are
// those of creating one.
func (s *Store) RenewInvitation(ctx context.Context, workspaceID, id string, hash []byte, actor model.Role) (Invitation, error) {
	if !ids.Valid(id) {
		return Invitation{}, ErrNotFound
	}
	now := s.now()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		managed, maxMembers, err := lockWorkspace(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		if managed {
			return ErrMembersManaged
		}
		var email, role string
		if err := tx.QueryRow(ctx, `SELECT email, role FROM invitations
			WHERE workspace_id = $1 AND id = $2 AND accepted_at IS NULL AND revoked_at IS NULL FOR UPDATE`,
			workspaceID, id).Scan(&email, &role); err != nil {
			return notFound(err)
		}
		if !model.CanInvite(actor, model.Role(role)) {
			return ErrRoleForbidden
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN users u ON u.id = m.user_id
			WHERE m.workspace_id = $1 AND lower(u.email) = $2)`, workspaceID, email).Scan(&member); err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		if isFull, err := full(ctx, tx, workspaceID, maxMembers); err != nil || isFull {
			if err == nil {
				err = ErrWorkspaceFull
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE invitations SET hash = $2, expires_at = $3 WHERE id = $1`,
			id, hash, now.Add(InvitationLifetime)); err != nil {
			return err
		}
		return s.record(ctx, tx, event{workspace: workspaceID, action: audit.InvitationRenewed, target: audit.TargetInvitation,
			targetID: id, label: email, details: map[string]any{"role": role}})
	})
	if err != nil {
		return Invitation{}, err
	}
	return scanInvitation(s.pool.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE i.id = $1`, id))
}
