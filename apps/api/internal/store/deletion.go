package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
)

var (
	// ErrConfirmationMismatch: the confirmation isn't the space's name.
	ErrConfirmationMismatch = errors.New("store: confirmation isn't the space's name")
	// ErrWorkspaceDelegated: other spaces use organizations of the space by
	// delegation.
	ErrWorkspaceDelegated = errors.New("store: workspace has delegations")
)

// DeletedSpaces is what deleting spaces removed that lives outside the
// database too: the hashes of the keys to drop from the cache and the
// certificates whose material to forget.
type DeletedSpaces struct {
	KeyHashes    [][]byte
	Certificates []string
	// Organizations is how many organizations went with the spaces.
	Organizations int
}

// deleteSpaces deletes the spaces ids with everything in them, in tx: their
// organizations (which don't cascade from their space: RESTRICT) with their
// certificates, their delegations and every key of only one of them, also
// another space's; and, with each space, its members, its billing seats,
// the delegations it was given, its keys, its invitations, what it signed
// in the log, its usage and its audit log. What other spaces signed with its
// organizations stays in their log and their usage: it's theirs. The
// caller has locked the spaces and checked nothing else uses their
// organizations.
func deleteSpaces(ctx context.Context, tx pgx.Tx, ids []string) (DeletedSpaces, error) {
	var out DeletedSpaces
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT hash FROM api_keys WHERE revoked_at IS NULL
		AND (workspace_id = ANY($1) OR organization_id IN (SELECT id FROM organizations WHERE workspace_id = ANY($1)))`, ids)
	if err != nil {
		return out, err
	}
	if out.KeyHashes, err = pgx.CollectRows(rows, pgx.RowTo[[]byte]); err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, `SELECT c.id::text FROM certificates c JOIN organizations o ON o.id = c.organization_id
		WHERE o.workspace_id = ANY($1)`, ids)
	if err != nil {
		return out, err
	}
	if out.Certificates, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return out, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM organizations WHERE workspace_id = ANY($1)`, ids)
	if err != nil {
		return out, err
	}
	out.Organizations = int(tag.RowsAffected())
	_, err = tx.Exec(ctx, `DELETE FROM workspaces WHERE id = ANY($1)`, ids)
	return out, err
}

// DeletedWorkspace is what deleting a space did.
type DeletedWorkspace struct {
	DeletedSpaces
	// Returned are the organizations that went back to the platform's
	// space instead of being deleted (returnToPlatform).
	Returned []string
}

// DeleteWorkspace deletes the space id with everything in it
// (deleteSpaces), if confirmation is its name, exactly
// (ErrConfirmationMismatch). Under a lock of the space and of its
// organizations, so that nothing is delegated or created in it meanwhile:
// first the organizations the platform's space (platformID, in the cloud)
// holds a locked delegation of go back to it, as when the space closes;
// then, if another space still uses one of its organizations by
// delegation, nothing changes (ErrWorkspaceDelegated). Who may delete it
// (its owner, never the platform's space nor one whose plan and members
// billing pushes) is the caller's to check.
func (s *Store) DeleteWorkspace(ctx context.Context, id, confirmation, platformID string) (DeletedWorkspace, error) {
	var out DeletedWorkspace
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		out = DeletedWorkspace{}
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM workspaces WHERE id = $1 FOR UPDATE`, id).Scan(&name); err != nil {
			return notFound(err)
		}
		if confirmation != name {
			return ErrConfirmationMismatch
		}
		// A delegation, a key of one organization or a certificate added
		// to one of them now waits for this transaction.
		if _, err := tx.Exec(ctx, `SELECT id FROM organizations WHERE workspace_id = $1 ORDER BY id FOR UPDATE`, id); err != nil {
			return err
		}
		// The platform's space is told the space gave them back, never who
		// deleted it.
		returned, revoked, err := s.returnToPlatform(ctx, tx, id, platformID, audit.Space(id))
		if err != nil {
			return err
		}
		out.Returned = returned
		var delegated bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM delegations d JOIN organizations o ON o.id = d.organization_id
			WHERE o.workspace_id = $1 AND d.workspace_id <> $1)`, id).Scan(&delegated); err != nil {
			return err
		}
		if delegated {
			return ErrWorkspaceDelegated
		}
		deleted, err := deleteSpaces(ctx, tx, []string{id})
		if err != nil {
			return err
		}
		out.DeletedSpaces = deleted
		out.KeyHashes = append(out.KeyHashes, revoked...)
		return nil
	})
	if err != nil {
		return DeletedWorkspace{}, err
	}
	return out, nil
}
