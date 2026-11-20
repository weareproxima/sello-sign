package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

// reachJoin holds an organization by the space $1: its own, or delegated
// to it (d.id isn't null).
const reachJoin = `FROM organizations o
	LEFT JOIN delegations d ON d.organization_id = o.id AND d.workspace_id = $1
	WHERE (o.workspace_id = $1 OR d.id IS NOT NULL)`

const organizationColumns = `o.id, o.workspace_id, o.name, o.tax_id, o.country, o.contact_email,
	o.sello_workspace_id, o.created_at, d.id, d.scopes, d.locked`

func scanOrganization(row pgx.Row) (Organization, error) {
	var o Organization
	var dID *string
	var dScopes []string
	var dLocked *bool
	if err := row.Scan(&o.ID, &o.WorkspaceID, &o.Name, &o.TaxID, &o.Country, &o.ContactEmail,
		&o.SelloWorkspaceID, &o.CreatedAt, &dID, &dScopes, &dLocked); err != nil {
		return Organization{}, notFound(err)
	}
	if dID != nil {
		o.Reach = model.Reach{Relation: model.Delegated, Scopes: model.ScopesOf(dScopes)}
		o.Delegation = &DelegationRef{ID: *dID, Scopes: o.Reach.Scopes, Locked: dLocked != nil && *dLocked}
	} else {
		o.Reach = model.Reach{Relation: model.Owned}
	}
	return o, nil
}

// OrganizationFilter narrows a space's organizations.
type OrganizationFilter struct {
	// Only is the one organization a key of a single organization sees.
	Only string
	// Relation is "owned", "delegated" or "" for both.
	Relation model.Relation
}

// Organizations lists the organizations the space reaches, newest first.
func (s *Store) Organizations(ctx context.Context, workspaceID string, f OrganizationFilter, p Page) ([]Organization, *string, error) {
	var only any
	if f.Only != "" {
		only = f.Only
	}
	rows, err := s.pool.Query(ctx, `SELECT `+organizationColumns+` `+reachJoin+`
		AND ($2::uuid IS NULL OR o.id = $2)
		AND ($3 = '' OR ($3 = 'owned' AND o.workspace_id = $1) OR ($3 = 'delegated' AND d.id IS NOT NULL))
		AND ($4::uuid IS NULL OR o.id < $4)
		ORDER BY o.id DESC LIMIT $5`, workspaceID, only, string(f.Relation), cursorArg(p), limitArg(p))
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Organization, error) { return scanOrganization(r) })
	if err != nil {
		return nil, nil, err
	}
	items, next := trim(items, p, func(o Organization) string { return o.ID })
	return items, next, nil
}

// ResolveOrganization finds an organization the space reaches by ref: its
// id or its tax id. only, if set, is the single organization a key may use.
// A tax id that names several (one per country) is ErrAmbiguous.
func (s *Store) ResolveOrganization(ctx context.Context, workspaceID, ref, only string) (Organization, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || !ids.Valid(workspaceID) {
		return Organization{}, ErrNotFound
	}
	var onlyArg any
	if only != "" {
		onlyArg = only
	}
	if id, ok := ids.Canonical(ref); ok {
		return scanOrganization(s.pool.QueryRow(ctx, `SELECT `+organizationColumns+` `+reachJoin+`
			AND o.id = $2 AND ($3::uuid IS NULL OR o.id = $3)`, workspaceID, id, onlyArg))
	}
	key := model.TaxIDKey(ref)
	if key == "" {
		return Organization{}, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT `+organizationColumns+` `+reachJoin+`
		AND o.tax_id_key = $2 AND ($3::uuid IS NULL OR o.id = $3)
		ORDER BY (o.workspace_id = $1) DESC LIMIT 3`, workspaceID, key, onlyArg)
	if err != nil {
		return Organization{}, err
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Organization, error) { return scanOrganization(r) })
	if err != nil {
		return Organization{}, err
	}
	switch {
	case len(found) == 0:
		return Organization{}, ErrNotFound
	case len(found) > 1:
		return Organization{}, ErrAmbiguous
	}
	return found[0], nil
}

// NewOrganization is what creates one.
type NewOrganization struct {
	WorkspaceID      string
	Name             string
	TaxID            string
	Country          string
	ContactEmail     *string
	SelloWorkspaceID *string
}

// CreateOrganization creates an organization in its space, or returns the
// one the space already has with that country and tax id (created false).
// A sello_workspace_id already linked elsewhere is ErrSelloLinked; a new
// one past the organizations the space's plan allows, ErrOrganizationLimit.
func (s *Store) CreateOrganization(ctx context.Context, n NewOrganization) (Organization, bool, error) {
	id := ids.New()
	key := model.TaxIDKey(n.TaxID)
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		allowed, err := lockOrganizationLimit(ctx, tx, n.WorkspaceID)
		if err != nil {
			return err
		}
		var existing string
		err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE workspace_id = $1 AND country = $2 AND tax_id_key = $3`,
			n.WorkspaceID, n.Country, key).Scan(&existing)
		if err == nil {
			id = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := withinOrganizationLimit(ctx, tx, n.WorkspaceID, allowed, 1); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO organizations
			(id, workspace_id, name, tax_id, tax_id_key, country, contact_email, sello_workspace_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, n.WorkspaceID, n.Name, strings.TrimSpace(n.TaxID), key, n.Country, n.ContactEmail, n.SelloWorkspaceID)
		if c, ok := uniqueViolation(err); ok && strings.Contains(c, "sello_workspace_id") {
			return ErrSelloLinked
		}
		if err != nil {
			return err
		}
		created = true
		return s.record(ctx, tx, event{workspace: n.WorkspaceID, action: audit.OrganizationCreated, target: audit.TargetOrganization,
			targetID: id, label: n.Name, details: map[string]any{
				"tax_id": strings.TrimSpace(n.TaxID), "country": n.Country, "contact_email": n.ContactEmail,
				"sello_workspace_id": n.SelloWorkspaceID,
			}})
	})
	if err != nil {
		return Organization{}, false, err
	}
	o, err := s.ResolveOrganization(ctx, n.WorkspaceID, id, "")
	return o, created, err
}

// lockOrganizationLimit reads how many organizations the space's plan
// allows (nil: no limit), locking its row so that two creations or claims
// at once can't both take the last one. FOR NO KEY UPDATE doesn't wait for
// the signatures being logged, which only hold a key share of it.
func lockOrganizationLimit(ctx context.Context, tx pgx.Tx, workspaceID string) (*int, error) {
	var allowed *int
	err := tx.QueryRow(ctx, `SELECT max_organizations FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, workspaceID).Scan(&allowed)
	return allowed, notFound(err)
}

// withinOrganizationLimit is ErrOrganizationLimit if adding more
// organizations to those the space owns passes allowed.
func withinOrganizationLimit(ctx context.Context, tx pgx.Tx, workspaceID string, allowed *int, more int) error {
	if allowed == nil {
		return nil
	}
	var owned int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE workspace_id = $1`, workspaceID).Scan(&owned); err != nil {
		return err
	}
	if owned+more > *allowed {
		return ErrOrganizationLimit
	}
	return nil
}

// DelegateLinked is the platform registering, after a company, the
// organization of Sello's IdP selloID: the company already has it in its
// own space. The platform's space (platformID) gets the locked delegation
// of it, or keeps the one it has, and reads it as delegated; no second
// organization is created. One that is in the platform's own space, or
// that no organization names, is ErrSelloLinked.
func (s *Store) DelegateLinked(ctx context.Context, platformID, selloID, taxID, country string) (Organization, error) {
	if !ids.Valid(platformID) {
		return Organization{}, ErrSelloLinked
	}
	var id string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var owner, name, gotTaxID, gotCountry string
		err := tx.QueryRow(ctx, `SELECT id, workspace_id, name, tax_id, country FROM organizations WHERE sello_workspace_id = $1 FOR UPDATE`,
			selloID).Scan(&id, &owner, &name, &gotTaxID, &gotCountry)
		if err == pgx.ErrNoRows || (err == nil && owner == platformID) {
			return ErrSelloLinked
		}
		if err != nil {
			return err
		}
		// The platform would sign with that organization's certificate: a
		// different taxpayer is a conflict to surface, not to paper over.
		if model.TaxIDKey(gotTaxID) != model.TaxIDKey(taxID) || !strings.EqualFold(gotCountry, country) {
			return ErrSelloMismatch
		}
		scopes := model.Strings(model.PlatformDelegationScopes)
		var had struct {
			id     string
			scopes []string
			locked bool
		}
		err = tx.QueryRow(ctx, `SELECT id, scopes, locked FROM delegations WHERE organization_id = $1 AND workspace_id = $2 FOR UPDATE`,
			id, platformID).Scan(&had.id, &had.scopes, &had.locked)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			had.id = ids.New()
		case err != nil:
			return err
		case had.locked && slices.Equal(had.scopes, scopes):
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO delegations (id, organization_id, workspace_id, scopes, locked)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (organization_id, workspace_id) DO UPDATE SET scopes = EXCLUDED.scopes, locked = true`,
			had.id, id, platformID, scopes); err != nil {
			return err
		}
		return s.recordDelegation(ctx, tx, audit.DelegationGranted, platformID, owner, id, name, delegationFacts{
			id: had.id, delegate: platformID, scopes: scopes, locked: true,
		})
	})
	if err != nil {
		return Organization{}, err
	}
	return s.ResolveOrganization(ctx, platformID, id, "")
}

// OrganizationPatch is what PATCH changes; nil leaves it, a nil inner
// pointer clears it.
type OrganizationPatch struct {
	Name             *string
	ContactEmail     **string
	SelloWorkspaceID **string
}

// UpdateOrganization changes an organization.
func (s *Store) UpdateOrganization(ctx context.Context, id string, p OrganizationPatch) error {
	var email, sello *string
	if p.ContactEmail != nil {
		email = *p.ContactEmail
	}
	if p.SelloWorkspaceID != nil {
		sello = *p.SelloWorkspaceID
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var owner, name string
		var oldEmail, oldSello *string
		if err := tx.QueryRow(ctx, `SELECT workspace_id, name, contact_email, sello_workspace_id FROM organizations
			WHERE id = $1 FOR UPDATE`, id).Scan(&owner, &name, &oldEmail, &oldSello); err != nil {
			return notFound(err)
		}
		_, err := tx.Exec(ctx, `UPDATE organizations SET
				name = COALESCE($2, name),
				contact_email = CASE WHEN $3 THEN $4 ELSE contact_email END,
				sello_workspace_id = CASE WHEN $5 THEN $6 ELSE sello_workspace_id END,
				updated_at = now()
			WHERE id = $1`, id, p.Name, p.ContactEmail != nil, email, p.SelloWorkspaceID != nil, sello)
		if c, ok := uniqueViolation(err); ok && strings.Contains(c, "sello_workspace_id") {
			return ErrSelloLinked
		}
		if err != nil {
			return err
		}
		changes := audit.Changes{}
		if p.Name != nil {
			changes.Set("name", name, *p.Name)
			name = *p.Name
		}
		if p.ContactEmail != nil {
			changes.Set("contact_email", oldEmail, email)
		}
		if p.SelloWorkspaceID != nil {
			changes.Set("sello_workspace_id", oldSello, sello)
		}
		if len(changes) == 0 {
			return nil
		}
		return s.record(ctx, tx, event{workspace: owner, action: audit.OrganizationUpdated, target: audit.TargetOrganization,
			targetID: id, label: name, details: map[string]any{"changes": changes}})
	})
}

// DeleteOrganization deletes an organization with its certificates and
// keys; one that is delegated is ErrHasDelegations. Its log and usage stay.
// Its keys, which go with it, are recorded as revoked in their spaces.
func (s *Store) DeleteOrganization(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var owner, name, taxID, country string
		if err := tx.QueryRow(ctx, `SELECT workspace_id, name, tax_id, country FROM organizations WHERE id = $1 FOR UPDATE`,
			id).Scan(&owner, &name, &taxID, &country); err != nil {
			return notFound(err)
		}
		var n, certificates int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM delegations WHERE organization_id = $1),
			(SELECT count(*) FROM certificates WHERE organization_id = $1)`, id).Scan(&n, &certificates); err != nil {
			return err
		}
		if n > 0 {
			return ErrHasDelegations
		}
		type gone struct{ id, workspace, name, env string }
		rows, err := tx.Query(ctx, `SELECT id, workspace_id, name, environment FROM api_keys
			WHERE organization_id = $1 AND revoked_at IS NULL ORDER BY id`, id)
		if err != nil {
			return err
		}
		keys, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (gone, error) {
			var k gone
			err := r.Scan(&k.id, &k.workspace, &k.name, &k.env)
			return k, err
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id); err != nil {
			return err
		}
		if err := s.record(ctx, tx, event{workspace: owner, action: audit.OrganizationDeleted, target: audit.TargetOrganization,
			targetID: id, label: name, details: map[string]any{"tax_id": taxID, "country": country, "certificates": certificates}}); err != nil {
			return err
		}
		for _, k := range keys {
			if err := s.record(ctx, tx, event{workspace: k.workspace, action: audit.APIKeyRevoked, target: audit.TargetAPIKey,
				targetID: k.id, label: k.name, details: map[string]any{
					"environment": k.env, "organization_id": id, "reason": "organization_deleted",
				}}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ClaimableOrganizations lists the organizations of the platform's space
// linked to the given organizations of Sello's IdP.
func (s *Store) ClaimableOrganizations(ctx context.Context, platformID string, selloIDs []string) ([]Organization, error) {
	if !ids.Valid(platformID) || len(selloIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+organizationColumns+` `+reachJoin+`
		AND o.workspace_id = $1 AND o.sello_workspace_id = ANY($2)
		ORDER BY o.id DESC`, platformID, selloIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Organization, error) { return scanOrganization(r) })
}

// ClaimOrganizations moves the organizations orgIDs from the platform's
// space to target, with their certificates and their history, and leaves
// the platform a locked delegation of each, all in one transaction. Each
// must be in the platform's space and linked to one of selloIDs (the
// organizations of the IdP the person manages); otherwise nothing moves
// (ErrNotClaimable). One whose country and tax id target already has is
// ErrOrganizationExists.
func (s *Store) ClaimOrganizations(ctx context.Context, target, platformID string, orgIDs, selloIDs []string) ([]Organization, error) {
	if target == platformID {
		return nil, ErrNotClaimable
	}
	for _, id := range orgIDs {
		if !ids.Valid(id) {
			return nil, ErrNotClaimable
		}
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		allowed, err := lockOrganizationLimit(ctx, tx, target)
		if err != nil {
			return err
		}
		type claimed struct{ id, name, taxID, country string }
		rows, err := tx.Query(ctx, `SELECT id, name, tax_id, country FROM organizations
			WHERE id = ANY($1) AND workspace_id = $2 AND sello_workspace_id = ANY($3)
			ORDER BY id FOR UPDATE`, orgIDs, platformID, selloIDs)
		if err != nil {
			return err
		}
		orgs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (claimed, error) {
			var o claimed
			err := r.Scan(&o.id, &o.name, &o.taxID, &o.country)
			return o, err
		})
		if err != nil {
			return err
		}
		if len(orgs) != len(dedupe(orgIDs)) {
			return ErrNotClaimable
		}
		found := make([]string, len(orgs))
		for i, o := range orgs {
			found[i] = o.id
		}
		if err := withinOrganizationLimit(ctx, tx, target, allowed, len(found)); err != nil {
			return err
		}
		// The target may have had a delegation of one: it owns it now.
		if _, err := tx.Exec(ctx, `DELETE FROM delegations WHERE workspace_id = $1 AND organization_id = ANY($2)`, target, found); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE organizations SET workspace_id = $1, updated_at = now() WHERE id = ANY($2)`, target, found)
		if c, ok := uniqueViolation(err); ok && strings.Contains(c, "workspace_id") {
			return ErrOrganizationExists
		}
		if err != nil {
			return err
		}
		// The platform's keys of a single organization keep working through
		// the delegation.
		if _, err := tx.Exec(ctx, `INSERT INTO delegations (id, organization_id, workspace_id, scopes, locked)
			SELECT x.id, x.org, $1, $3, true FROM unnest($4::uuid[], $2::uuid[]) AS x(id, org)
			ON CONFLICT (organization_id, workspace_id) DO UPDATE SET scopes = EXCLUDED.scopes, locked = true`,
			platformID, found, model.Strings(model.PlatformDelegationScopes), newIDs(len(found))); err != nil {
			return err
		}
		// In the space that brings them, with its person; in the platform's,
		// with that space as the actor.
		from, err := workspaceName(ctx, tx, platformID)
		if err != nil {
			return err
		}
		to, err := workspaceName(ctx, tx, target)
		if err != nil {
			return err
		}
		by := audit.Space(target)
		for _, o := range orgs {
			e := event{workspace: target, action: audit.OrganizationClaimed, target: audit.TargetOrganization, targetID: o.id,
				label: o.name, details: map[string]any{
					"tax_id": o.taxID, "country": o.country, "from_workspace_id": platformID, "from_workspace_name": from,
					"to_workspace_id": target, "to_workspace_name": to,
				}}
			if err := s.record(ctx, tx, e); err != nil {
				return err
			}
			e.workspace, e.actor = platformID, &by
			if err := s.record(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(orgIDs))
	for _, id := range dedupe(orgIDs) {
		o, err := s.ResolveOrganization(ctx, target, id, "")
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		c, _ := ids.Canonical(s)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func newIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = ids.New()
	}
	return out
}
