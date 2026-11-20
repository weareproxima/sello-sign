package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/model"
)

// The notices the API e-mails every hour (internal/notices): what each one
// reads, and the rows that make it go once (migration 0004). The spaces
// that get notices are the open ones (not closed in Sello) other than the
// platform's (except): its organizations' certificates are Sello's
// platform's to warn of, and it has no plan.

// ExpiringCertificate is an active certificate that expires soon, or
// expired while active, with what a notice says of it.
type ExpiringCertificate struct {
	ID               string
	Environment      model.Environment
	NotAfter         time.Time
	OrganizationID   string
	OrganizationName string
	// WorkspaceID is the space that owns the organization: the one that
	// uploads its certificates, and the one told.
	WorkspaceID   string
	WorkspaceName string
	// Sent are the thresholds already notified.
	Sent []string
}

// ExpiringCertificates lists the active certificates whose not_after is
// at or before `before`, of the organizations of the spaces that get notices
// (the platform's, except, doesn't), with the thresholds already notified;
// by space, then by when they expire.
func (s *Store) ExpiringCertificates(ctx context.Context, before time.Time, except string) ([]ExpiringCertificate, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.id, c.environment, c.not_after, o.id, o.name, w.id, w.name,
			COALESCE(array_agg(n.threshold ORDER BY n.threshold) FILTER (WHERE n.threshold IS NOT NULL), '{}')
		FROM certificates c
		JOIN organizations o ON o.id = c.organization_id
		JOIN workspaces w ON w.id = o.workspace_id
		LEFT JOIN certificate_notices n ON n.certificate_id = c.id
		WHERE c.active AND c.not_after IS NOT NULL AND c.not_after <= $1
			AND w.billing_closed_at IS NULL AND w.id::text <> $2
		GROUP BY c.id, o.id, w.id
		ORDER BY w.id, c.not_after, c.id`, before, except)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExpiringCertificate, error) {
		var c ExpiringCertificate
		var env string
		err := r.Scan(&c.ID, &env, &c.NotAfter, &c.OrganizationID, &c.OrganizationName, &c.WorkspaceID, &c.WorkspaceName, &c.Sent)
		c.Environment = model.Environment(env)
		return c, err
	})
}

// CertificateNotice is a certificate's threshold, notified.
type CertificateNotice struct {
	CertificateID string
	Threshold     string
}

// MarkCertificateNotices records that these thresholds were notified; one
// already recorded stays as it was.
func (s *Store) MarkCertificateNotices(ctx context.Context, sent []CertificateNotice) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, n := range sent {
			// A certificate deleted since it was read: nothing to record.
			if _, err := tx.Exec(ctx, `INSERT INTO certificate_notices (certificate_id, threshold)
				SELECT id, $2 FROM certificates WHERE id = $1
				ON CONFLICT (certificate_id, threshold) DO NOTHING`, n.CertificateID, n.Threshold); err != nil {
				return err
			}
		}
		return nil
	})
}

// SpaceUsage is what a space that has a monthly limit in an environment
// signed there in a month.
type SpaceUsage struct {
	WorkspaceID string
	Name        string
	// Plan is the cloud's plan; nil for a space whose limits are the
	// install's (the console's).
	Plan        *string
	Environment model.Environment
	Used        int64
	// Quota is what the plan includes in the month (max_*_signatures),
	// and Cap where signing stops (*_hard_cap); nil is none.
	Quota *int
	Cap   *int
}

// NoticeUsage lists what each space that gets notices (not except) and
// has a monthly limit in an environment signed there in month, when it
// signed something.
func (s *Store) NoticeUsage(ctx context.Context, month, except string) ([]SpaceUsage, error) {
	rows, err := s.pool.Query(ctx, `SELECT w.id, w.name, w.plan, u.environment, u.count,
			CASE u.environment WHEN 'prod' THEN w.max_prod_signatures ELSE w.max_test_signatures END,
			CASE u.environment WHEN 'prod' THEN w.prod_hard_cap ELSE w.test_hard_cap END
		FROM workspace_usage u JOIN workspaces w ON w.id = u.workspace_id
		WHERE u.month = $1 AND u.count > 0 AND w.billing_closed_at IS NULL AND w.id::text <> $2
			AND CASE u.environment
				WHEN 'prod' THEN COALESCE(w.max_prod_signatures, w.prod_hard_cap)
				ELSE COALESCE(w.max_test_signatures, w.test_hard_cap) END IS NOT NULL
		ORDER BY w.id, u.environment`, month, except)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SpaceUsage, error) {
		var u SpaceUsage
		var env string
		err := r.Scan(&u.WorkspaceID, &u.Name, &u.Plan, &env, &u.Used, &u.Quota, &u.Cap)
		u.Environment = model.Environment(env)
		return u, err
	})
}

// UsageNotice is a threshold of a space's usage in an environment and a
// month, measured against Limit.
type UsageNotice struct {
	WorkspaceID string
	Environment model.Environment
	Month       string
	Threshold   string
	Limit       int
}

// UsageNoticesSent are the usage thresholds notified in month.
func (s *Store) UsageNoticesSent(ctx context.Context, month string) ([]UsageNotice, error) {
	rows, err := s.pool.Query(ctx, `SELECT workspace_id, environment, month, threshold, limit_value
		FROM usage_notices WHERE month = $1`, month)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (UsageNotice, error) {
		var n UsageNotice
		var env string
		err := r.Scan(&n.WorkspaceID, &env, &n.Month, &n.Threshold, &n.Limit)
		n.Environment = model.Environment(env)
		return n, err
	})
}

// MarkUsageNotice records that a usage threshold was notified; if it
// already was, it stays as it was.
func (s *Store) MarkUsageNotice(ctx context.Context, n UsageNotice) error {
	// A space deleted since it was read: nothing to record.
	_, err := s.pool.Exec(ctx, `INSERT INTO usage_notices (workspace_id, environment, month, threshold, limit_value)
		SELECT id, $2, $3, $4, $5 FROM workspaces WHERE id = $1
		ON CONFLICT (workspace_id, environment, month, threshold, limit_value) DO NOTHING`,
		n.WorkspaceID, string(n.Environment), n.Month, n.Threshold, n.Limit)
	return err
}

// PurgeUsageNotices deletes the usage notices of the months before
// `before` ("YYYY-MM"): they're only read in their month.
func (s *Store) PurgeUsageNotices(ctx context.Context, before string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM usage_notices WHERE month < $1`, before)
	return tag.RowsAffected(), err
}

// NoticeRecipients are the addresses of who administers a space (its
// owners and admins: those added in Sign and, in the cloud, the seats
// billing pushed that the plan admits), as the panel's last token of each
// said; without repeats, in order. Who never signed in to the panel has no
// address here, and isn't told.
func (s *Store) NoticeRecipients(ctx context.Context, workspaceID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.email FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 AND m.role IN ('owner', 'admin') AND u.email <> ''
		UNION
		SELECT u.email FROM billing_members bm JOIN workspaces w ON w.id = bm.workspace_id
		JOIN users u ON u.sello_sub = bm.account
		WHERE bm.workspace_id = $1 AND bm.role IN ('owner', 'admin') AND u.email <> ''
			AND (w.max_members IS NULL OR bm.position < w.max_members)
		ORDER BY 1`, workspaceID)
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, e := range all {
		e = strings.TrimSpace(e)
		k := strings.ToLower(e)
		if e == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out, nil
}
