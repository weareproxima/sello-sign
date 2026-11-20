package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

// ResolveOrganizationForPerson finds an organization by ref (its id or its
// tax id) among those the person reaches through any of their spaces: the
// owner's, or one it's delegated to. Through more than one, the owner's
// wins, then a delegation that signs. A tax id that names several
// organizations is ErrAmbiguous. space is the space that reaches it.
func (s *Store) ResolveOrganizationForPerson(ctx context.Context, userID, selloSub, ref string) (org Organization, space string, err error) {
	var id any
	key := ""
	if canonical, ok := ids.Canonical(ref); ok {
		id = canonical
	} else if key = model.TaxIDKey(ref); key == "" {
		return Organization{}, "", ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (o.id) `+organizationColumns+`, x.workspace_id
		FROM organizations o
		JOIN (`+membershipQuery+`) x ON true
		LEFT JOIN delegations d ON d.organization_id = o.id AND d.workspace_id = x.workspace_id
		WHERE (o.workspace_id = x.workspace_id OR d.id IS NOT NULL)
		AND ($3::uuid IS NULL OR o.id = $3) AND ($4 = '' OR o.tax_id_key = $4)
		ORDER BY o.id, (o.workspace_id = x.workspace_id) DESC, (d.scopes @> ARRAY['sign']) DESC NULLS LAST
		LIMIT 3`, userID, selloSub, id, key)
	if err != nil {
		return Organization{}, "", err
	}
	type found struct {
		org   Organization
		space string
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (found, error) {
		var f found
		var dID *string
		var dScopes []string
		var dLocked *bool
		o := &f.org
		if err := r.Scan(&o.ID, &o.WorkspaceID, &o.Name, &o.TaxID, &o.Country, &o.ContactEmail,
			&o.SelloWorkspaceID, &o.CreatedAt, &dID, &dScopes, &dLocked, &f.space); err != nil {
			return f, err
		}
		if dID != nil && o.WorkspaceID != f.space {
			o.Reach = model.Reach{Relation: model.Delegated, Scopes: model.ScopesOf(dScopes)}
			o.Delegation = &DelegationRef{ID: *dID, Scopes: o.Reach.Scopes, Locked: dLocked != nil && *dLocked}
		} else {
			o.Reach = model.Reach{Relation: model.Owned}
		}
		return f, nil
	})
	switch {
	case err != nil:
		return Organization{}, "", err
	case len(all) == 0:
		return Organization{}, "", ErrNotFound
	case len(all) > 1:
		return Organization{}, "", ErrAmbiguous
	}
	return all[0].org, all[0].space, nil
}

// AllOrganizations lists every organization the space reaches, newest
// first.
func (s *Store) AllOrganizations(ctx context.Context, workspaceID string) ([]Organization, error) {
	var out []Organization
	p := Page{Limit: MaxLimit}
	for {
		items, next, err := s.Organizations(ctx, workspaceID, OrganizationFilter{}, p)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == nil {
			return out, nil
		}
		p.Cursor = *next
	}
}

// ExportedKey is a key as an export gives it: also a revoked one, with
// who created it; never its hash.
type ExportedKey struct {
	APIKey
	CreatedBy *string
	RevokedAt *time.Time
}

// AllAPIKeys lists every key of a space, revoked ones included, newest
// first.
func (s *Store) AllAPIKeys(ctx context.Context, workspaceID string) ([]ExportedKey, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+keyColumns+`, k.created_by, k.revoked_at FROM api_keys k
		WHERE k.workspace_id = $1 ORDER BY k.id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportedKey, error) {
		var e ExportedKey
		var env string
		var scopes []string
		k := &e.APIKey
		err := r.Scan(&k.ID, &k.WorkspaceID, &k.OrganizationID, &env, &k.Name, &scopes, &k.Prefix,
			&k.RequestsPerMinute, &k.LastUsedAt, &k.CreatedAt, &e.CreatedBy, &e.RevokedAt)
		k.Environment = model.Environment(env)
		k.Scopes = model.ScopesOf(scopes)
		return e, err
	})
}

// exportBatch is how many entries of the log an export reads at a time.
const exportBatch = 1000

// EachSignature calls fn with every entry of the log a space sees (what
// GET /workspaces/{w}/signatures pages through), newest first, reading it in
// batches so that no connection is held while fn writes.
func (s *Store) EachSignature(ctx context.Context, workspaceID string, fn func(SignatureEntry) error) error {
	var cursor any
	for {
		rows, err := s.pool.Query(ctx, signaturesOf+` AND ($2::uuid IS NULL OR l.id < $2) ORDER BY l.id DESC LIMIT $3`,
			workspaceID, cursor, exportBatch)
		if err != nil {
			return err
		}
		batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureEntry, error) { return scanSignature(r) })
		if err != nil {
			return err
		}
		for _, e := range batch {
			if err := fn(e); err != nil {
				return err
			}
		}
		if len(batch) < exportBatch {
			return nil
		}
		cursor = batch[len(batch)-1].ID
	}
}
