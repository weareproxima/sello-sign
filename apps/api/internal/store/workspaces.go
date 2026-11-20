package store

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

const workspaceColumns = `w.id, w.name, w.default_country, w.sello_workspace_id, w.plan,
	w.max_organizations, w.max_prod_signatures, w.max_test_signatures, w.max_members,
	w.requests_per_minute, w.retention_days, w.prod_hard_cap, w.test_hard_cap,
	w.billing_version, w.billing_closed_at, w.created_at, w.own_requests_per_minute`

// scanWorkspace reads a space; one without a plan takes the install's
// defaults where its row has no limit.
func (s *Store) scanWorkspace(row pgx.Row, extra ...any) (Workspace, error) {
	var w Workspace
	l := &w.Limits
	dest := append([]any{&w.ID, &w.Name, &w.DefaultCountry, &w.SelloWorkspaceID, &w.Plan,
		&l.Organizations, &l.ProdSignatures, &l.TestSignatures, &l.Members,
		&l.RequestsPerMinute, &l.RetentionDays, &l.ProdHardCap, &l.TestHardCap,
		&w.BillingVersion, &w.ClosedAt, &w.CreatedAt, &w.OwnRequestsPerMinute}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Workspace{}, notFound(err)
	}
	if w.Plan == nil {
		if l.RequestsPerMinute == nil {
			l.RequestsPerMinute = s.defaults.RequestsPerMinute
		}
		if l.RetentionDays == nil {
			l.RetentionDays = s.defaults.RetentionDays
		}
	}
	return w, nil
}

// UpsertUser keeps who a panel user is, as their token says, to name them
// among the members. A Sello account names one user: if another had it, it
// loses it.
func (s *Store) UpsertUser(ctx context.Context, id, email, name, selloSub string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var sub any
		if selloSub != "" {
			sub = selloSub
			if _, err := tx.Exec(ctx, `UPDATE users SET sello_sub = NULL WHERE sello_sub = $1 AND id <> $2`, selloSub, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO users (id, email, name, sello_sub) VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, name = EXCLUDED.name,
				sello_sub = EXCLUDED.sello_sub, updated_at = now()
			WHERE (users.email, users.name, users.sello_sub) IS DISTINCT FROM (EXCLUDED.email, EXCLUDED.name, EXCLUDED.sello_sub)`,
			id, email, name, sub)
		return err
	})
}

// CreateWorkspace creates a self-hosted space whose owner is ownerUserID
// (none if empty: a space made from the console).
func (s *Store) CreateWorkspace(ctx context.Context, name string, defaultCountry *string, ownerUserID string) (Workspace, error) {
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO workspaces (id, name, default_country) VALUES ($1, $2, $3)`,
			id, name, defaultCountry); err != nil {
			return err
		}
		if ownerUserID == "" {
			return nil
		}
		_, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, id, ownerUserID)
		return err
	})
	if err != nil {
		return Workspace{}, err
	}
	return s.Workspace(ctx, id)
}

// AddMember makes userID a member of a space with role, or changes their
// role.
func (s *Store) AddMember(ctx context.Context, workspaceID, userID string, role model.Role) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role`, workspaceID, userID, string(role))
	return err
}

// Workspace reads a space.
func (s *Store) Workspace(ctx context.Context, id string) (Workspace, error) {
	if !ids.Valid(id) {
		return Workspace{}, ErrNotFound
	}
	return s.scanWorkspace(s.pool.QueryRow(ctx, `SELECT `+workspaceColumns+` FROM workspaces w WHERE w.id = $1`, id))
}

// WorkspaceBySelloID reads the space of an organization of Sello's IdP.
func (s *Store) WorkspaceBySelloID(ctx context.Context, selloID string) (Workspace, error) {
	return s.scanWorkspace(s.pool.QueryRow(ctx, `SELECT `+workspaceColumns+` FROM workspaces w WHERE w.sello_workspace_id = $1`, selloID))
}

// WorkspacesBySelloIDs reads the spaces of several organizations of
// Sello's IdP, by their id there.
func (s *Store) WorkspacesBySelloIDs(ctx context.Context, selloIDs []string) (map[string]Workspace, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workspaceColumns+` FROM workspaces w WHERE w.sello_workspace_id = ANY($1)`, selloIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Workspace{}
	for rows.Next() {
		w, err := s.scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out[*w.SelloWorkspaceID] = w
	}
	return out, rows.Err()
}

// WorkspacePatch is what PATCH /workspaces/{w} changes; nil leaves it.
type WorkspacePatch struct {
	Name *string
	// DefaultCountry set to a nil *string clears it.
	DefaultCountry **string
	// OwnRequestsPerMinute set to a nil *int clears it.
	OwnRequestsPerMinute **int
}

// UpdateWorkspace changes a space.
func (s *Store) UpdateWorkspace(ctx context.Context, id string, p WorkspacePatch) (Workspace, error) {
	setCountry := p.DefaultCountry != nil
	var country *string
	if setCountry {
		country = *p.DefaultCountry
	}
	setRate := p.OwnRequestsPerMinute != nil
	var rate *int
	if setRate {
		rate = *p.OwnRequestsPerMinute
	}
	tag, err := s.pool.Exec(ctx, `UPDATE workspaces SET
			name = COALESCE($2, name),
			default_country = CASE WHEN $3 THEN $4 ELSE default_country END,
			own_requests_per_minute = CASE WHEN $5 THEN $6 ELSE own_requests_per_minute END,
			updated_at = now()
		WHERE id = $1`, id, p.Name, setCountry, country, setRate, rate)
	if err != nil {
		return Workspace{}, err
	}
	if tag.RowsAffected() == 0 {
		return Workspace{}, ErrNotFound
	}
	return s.Workspace(ctx, id)
}

// membershipQuery lists a person's roles: by their panel user id, and in
// the cloud by their Sello account among the seats billing pushed that
// the plan admits.
const membershipQuery = `
	SELECT m.workspace_id, m.role FROM workspace_members m WHERE m.user_id = $1
	UNION ALL
	SELECT bm.workspace_id, bm.role FROM billing_members bm JOIN workspaces w ON w.id = bm.workspace_id
	WHERE $2 <> '' AND bm.account = $2 AND (w.max_members IS NULL OR bm.position < w.max_members)`

// RoleIn is a person's role in a space, or "" if they aren't in it.
func (s *Store) RoleIn(ctx context.Context, workspaceID, userID, selloSub string) (model.Role, error) {
	if !ids.Valid(workspaceID) {
		return "", nil
	}
	rows, err := s.pool.Query(ctx, `SELECT role FROM (`+membershipQuery+`) x WHERE workspace_id = $3`, userID, selloSub, workspaceID)
	if err != nil {
		return "", err
	}
	roles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	var role model.Role
	for _, r := range roles {
		role = model.Higher(role, model.Role(r))
	}
	return role, nil
}

// WorkspaceWithRole is a space and the person's role in it.
type WorkspaceWithRole struct {
	Workspace
	Role model.Role
}

// WorkspacesOf lists the spaces a person is in, oldest first.
func (s *Store) WorkspacesOf(ctx context.Context, userID, selloSub string) ([]WorkspaceWithRole, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workspaceColumns+`, x.role
		FROM (`+membershipQuery+`) x JOIN workspaces w ON w.id = x.workspace_id
		ORDER BY w.id`, userID, selloSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspaceWithRole
	index := map[string]int{}
	for rows.Next() {
		var role string
		w, err := s.scanWorkspace(rows, &role)
		if err != nil {
			return nil, err
		}
		if i, ok := index[w.ID]; ok {
			out[i].Role = model.Higher(out[i].Role, model.Role(role))
			continue
		}
		index[w.ID] = len(out)
		out = append(out, WorkspaceWithRole{Workspace: w, Role: model.Role(role)})
	}
	return out, rows.Err()
}

// Members lists the people of a space: those added in Sign and, in the
// cloud, the seats billing pushed that the plan admits (with their panel
// user once they signed in).
func (s *Store) Members(ctx context.Context, workspaceID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.user_id, u.email, u.name, m.role, 0 AS source, 0 AS position
		FROM workspace_members m LEFT JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1
		UNION ALL
		SELECT u.id, u.email, u.name, bm.role, 1, bm.position
		FROM billing_members bm JOIN workspaces w ON w.id = bm.workspace_id
		LEFT JOIN users u ON u.sello_sub = bm.account
		WHERE bm.workspace_id = $1 AND (w.max_members IS NULL OR bm.position < w.max_members)
		ORDER BY 5, 6, 1`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	seen := map[string]int{}
	for rows.Next() {
		var m Member
		var role string
		var source, position int
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &role, &source, &position); err != nil {
			return nil, err
		}
		m.Role = model.Role(role)
		if m.UserID != nil {
			if i, ok := seen[*m.UserID]; ok {
				out[i].Role = model.Higher(out[i].Role, m.Role)
				continue
			}
			seen[*m.UserID] = len(out)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// normalizeCountry is a country as stored: upper case, nil if empty.
func normalizeCountry(c *string) *string {
	if c == nil || strings.TrimSpace(*c) == "" {
		return nil
	}
	u := strings.ToUpper(strings.TrimSpace(*c))
	return &u
}
