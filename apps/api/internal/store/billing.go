package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/billing"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

// CreateCloudWorkspace creates the space of an organization of Sello's IdP
// from the state billing answered when activating it, or finds the one
// that exists (created false). It starts one version behind, so applying
// the same state takes it whole, members included.
func (s *Store) CreateCloudWorkspace(ctx context.Context, st billing.State, name string) (string, bool, error) {
	if st.Name != "" {
		name = st.Name
	}
	var country *string
	if st.Country != nil && model.ValidCountry(*st.Country) {
		country = st.Country
	}
	id := ids.New()
	tag, err := s.pool.Exec(ctx, `INSERT INTO workspaces (id, name, default_country, sello_workspace_id, plan, billing_version)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (sello_workspace_id) DO NOTHING`,
		id, name, country, st.WorkspaceID, st.Plan, st.Version-1)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 1 {
		return id, true, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT id FROM workspaces WHERE sello_workspace_id = $1`, st.WorkspaceID).Scan(&id)
	return id, false, notFound(err)
}

// StateResult is what applying a pushed state did.
type StateResult struct {
	Applied billing.Applied
	// WorkspaceID is the space's id, unless Applied is AppliedUnknown.
	WorkspaceID string
	// Returned are the organizations a closed state sent back to the
	// platform's space.
	Returned []string
	// RevokedKeys are the hashes of the keys that closing revoked, to drop
	// them from the cache.
	RevokedKeys [][]byte
}

// ApplyState applies a state billing pushed to the space of its
// organization: the plan and its limits, the subscription, the name, the
// members, the closing. A version not greater than the one applied changes
// nothing. A closed organization keeps its members (its owner still reads
// its data) and its space stops signing; in the same transaction, the
// organizations of the space that the platform's space (platformID) holds
// a locked delegation of go back to it (returnToPlatform).
func (s *Store) ApplyState(ctx context.Context, st billing.State, platformID string) (StateResult, error) {
	var id string
	var version int64
	err := s.pool.QueryRow(ctx, `SELECT id, billing_version FROM workspaces WHERE sello_workspace_id = $1`, st.WorkspaceID).
		Scan(&id, &version)
	if err == pgx.ErrNoRows {
		return StateResult{Applied: billing.AppliedUnknown}, nil
	}
	if err != nil {
		return StateResult{}, err
	}
	res := StateResult{Applied: billing.AppliedStale, WorkspaceID: id}
	if st.Version <= version {
		return res, nil
	}
	applied := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		res.Returned, res.RevokedKeys = nil, nil
		var status, interval any
		var periodEnd any
		cancel := false
		if sub := st.Subscription; sub != nil {
			status, cancel = sub.Status, sub.CancelAtPeriodEnd
			if sub.Interval != nil {
				interval = *sub.Interval
			}
			if sub.CurrentPeriodEnd != nil {
				periodEnd = *sub.CurrentPeriodEnd
			}
		}
		var name any
		if !st.Closed && st.Name != "" {
			name = st.Name
		}
		l := st.Limits
		tag, err := tx.Exec(ctx, `UPDATE workspaces SET
				billing_version = $2, plan = $3,
				max_organizations = $4, max_prod_signatures = $5, max_test_signatures = $6, max_members = $7,
				requests_per_minute = $8, retention_days = $9, prod_hard_cap = $10, test_hard_cap = $11,
				subscription_status = $12, subscription_interval = $13, subscription_period_end = $14,
				subscription_cancel_at_period_end = $15,
				billing_closed_at = CASE WHEN $16 THEN COALESCE(billing_closed_at, now()) END,
				name = COALESCE($17, name),
				updated_at = now()
			WHERE id = $1 AND billing_version < $2`,
			id, st.Version, st.Plan, l.Issuers, l.ProdSignatures, l.TestSignatures, l.Members,
			l.RequestsPerMinute, l.RetentionDays, st.ProdHardCap, st.TestHardCap,
			status, interval, periodEnd, cancel, st.Closed, name)
		if err != nil {
			return err
		}
		// Two pushes at once: the greater version wins.
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		if st.Closed {
			res.Returned, res.RevokedKeys, err = returnToPlatform(ctx, tx, id, platformID)
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM billing_members WHERE workspace_id = $1`, id); err != nil {
			return err
		}
		for i, m := range st.Members {
			if _, err := tx.Exec(ctx, `INSERT INTO billing_members (workspace_id, account, role, position)
				VALUES ($1, $2, $3, $4) ON CONFLICT (workspace_id, account) DO NOTHING`,
				id, m.Account, string(m.Role), i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StateResult{}, err
	}
	if applied {
		res.Applied = billing.AppliedNew
	}
	return res, nil
}

// returnToPlatform sends back to the platform's space the organizations of
// the closing space that it holds a locked delegation of (those a company
// took with «Usar en Sello Sign» or that the platform registered after
// it): the platform owns each again, with its certificates and its
// history; its locked delegation goes, and so do the delegations the
// closing space had granted of it (the platform never granted them); the
// closing space's keys of only that organization are revoked. One whose
// country and tax id the platform's space already has stays, with the
// delegation. Running it again finds nothing to move.
func returnToPlatform(ctx context.Context, tx pgx.Tx, workspaceID, platformID string) ([]string, [][]byte, error) {
	if platformID == "" || platformID == workspaceID || !ids.Valid(platformID) {
		return nil, nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT o.id FROM organizations o
		JOIN delegations d ON d.organization_id = o.id AND d.workspace_id = $2 AND d.locked
		WHERE o.workspace_id = $1
		AND NOT EXISTS (SELECT 1 FROM organizations p
			WHERE p.workspace_id = $2 AND p.country = o.country AND p.tax_id_key = o.tax_id_key)
		ORDER BY o.id FOR UPDATE OF o`, workspaceID, platformID)
	if err != nil {
		return nil, nil, err
	}
	moved, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(moved) == 0 {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM delegations WHERE organization_id = ANY($1)`, moved); err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `UPDATE api_keys SET revoked_at = now()
		WHERE workspace_id = $1 AND organization_id = ANY($2) AND revoked_at IS NULL RETURNING hash`, workspaceID, moved)
	if err != nil {
		return nil, nil, err
	}
	hashes, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE organizations SET workspace_id = $1, updated_at = now() WHERE id = ANY($2)`,
		platformID, moved); err != nil {
		return nil, nil, err
	}
	return moved, hashes, nil
}

// UsageReport sums, for each space of the cloud that isn't closed, what it
// signed in each of months, as billing takes it.
func (s *Store) UsageReport(ctx context.Context, months []string) ([]billing.UsageEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT w.sello_workspace_id, u.month,
			COALESCE(sum(u.count) FILTER (WHERE u.environment = 'prod'), 0)::bigint,
			COALESCE(sum(u.count) FILTER (WHERE u.environment = 'test'), 0)::bigint
		FROM usage_counters u JOIN workspaces w ON w.id = u.workspace_id
		WHERE u.month = ANY($1) AND w.sello_workspace_id IS NOT NULL AND w.billing_closed_at IS NULL
		GROUP BY w.sello_workspace_id, u.month ORDER BY 1, 2`, months)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (billing.UsageEntry, error) {
		var e billing.UsageEntry
		err := r.Scan(&e.WorkspaceID, &e.Period, &e.Prod, &e.Test)
		return e, err
	})
}

// TryLock runs fn holding a session advisory lock if no other instance
// holds it; ran is false if one did.
func (s *Store) TryLock(ctx context.Context, key int64, fn func(context.Context) error) (ran bool, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, key) }()
	return true, fn(ctx)
}
