package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

// NewSignature is a signature to log and count.
type NewSignature struct {
	OrganizationID      string
	SignedByWorkspaceID string
	APIKeyID            string
	UserID              string
	Environment         model.Environment
	DocumentID          string
	DocumentType        string
	Duration            time.Duration
	At                  time.Time
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RecordSignature logs a signature and counts it in the signing space's
// month, in one transaction. It returns the log entry's id.
func (s *Store) RecordSignature(ctx context.Context, n NewSignature) (string, error) {
	at := n.At
	if at.IsZero() {
		at = s.now()
	}
	id := ids.NewAt(at).String()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO signing_log
			(id, organization_id, signed_by_workspace_id, api_key_id, user_id, environment, document_id, document_type, duration_ms, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, n.OrganizationID, n.SignedByWorkspaceID, nullable(n.APIKeyID), nullable(n.UserID), string(n.Environment),
			n.DocumentID, n.DocumentType, int(n.Duration.Milliseconds()), at); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO usage_counters (workspace_id, organization_id, environment, month, count)
			VALUES ($1, $2, $3, $4, 1)
			ON CONFLICT (workspace_id, organization_id, environment, month) DO UPDATE SET count = usage_counters.count + 1`,
			n.SignedByWorkspaceID, n.OrganizationID, string(n.Environment), model.Month(at))
		return err
	})
	return id, err
}

// SignatureFilter narrows the log.
type SignatureFilter struct {
	// Only is the one organization a key of a single organization sees.
	Only           string
	OrganizationID string
	Environment    model.Environment
	From, To       *time.Time
}

// signaturesOf is the log a space sees: what it signed, and what others
// signed with the organizations it owns. What another space signed comes
// without its key or its user.
const signaturesOf = `SELECT l.id, l.organization_id, o.name, l.environment, l.document_id, l.document_type,
		CASE WHEN l.signed_by_workspace_id = $1 THEN l.api_key_id END,
		CASE WHEN l.signed_by_workspace_id = $1 THEN l.user_id END,
		l.signed_by_workspace_id, l.duration_ms, l.created_at
	FROM signing_log l LEFT JOIN organizations o ON o.id = l.organization_id
	WHERE (l.signed_by_workspace_id = $1 OR o.workspace_id = $1)`

func scanSignature(row pgx.Row) (SignatureEntry, error) {
	var e SignatureEntry
	var env string
	if err := row.Scan(&e.ID, &e.OrganizationID, &e.OrganizationName, &env, &e.DocumentID, &e.DocumentType,
		&e.APIKeyID, &e.UserID, &e.SignedByWorkspaceID, &e.DurationMS, &e.CreatedAt); err != nil {
		return SignatureEntry{}, notFound(err)
	}
	e.Environment = model.Environment(env)
	return e, nil
}

// Signatures lists the log a space sees, newest first.
func (s *Store) Signatures(ctx context.Context, workspaceID string, f SignatureFilter, p Page) ([]SignatureEntry, *string, error) {
	rows, err := s.pool.Query(ctx, signaturesOf+`
		AND ($2::uuid IS NULL OR l.organization_id = $2)
		AND ($3::uuid IS NULL OR l.organization_id = $3)
		AND ($4 = '' OR l.environment = $4)
		AND ($5::timestamptz IS NULL OR l.created_at >= $5)
		AND ($6::timestamptz IS NULL OR l.created_at < $6)
		AND ($7::uuid IS NULL OR l.id < $7)
		ORDER BY l.id DESC LIMIT $8`,
		workspaceID, nullable(f.Only), nullable(f.OrganizationID), string(f.Environment), f.From, f.To, cursorArg(p), limitArg(p))
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureEntry, error) { return scanSignature(r) })
	if err != nil {
		return nil, nil, err
	}
	items, next := trim(items, p, func(e SignatureEntry) string { return e.ID })
	return items, next, nil
}

// Signature reads one entry of the log a space sees.
func (s *Store) Signature(ctx context.Context, workspaceID, id, only string) (SignatureEntry, error) {
	if !ids.Valid(id) {
		return SignatureEntry{}, ErrNotFound
	}
	return scanSignature(s.pool.QueryRow(ctx, signaturesOf+` AND l.id = $2 AND ($3::uuid IS NULL OR l.organization_id = $3)`,
		workspaceID, id, nullable(only)))
}

// UsageLine is what a space signed in a month and environment.
type UsageLine struct {
	Month       string
	Environment model.Environment
	Signatures  int64
}

// Usage sums what a space signed by month and environment, from month
// from to month to (both "YYYY-MM", inclusive), newest first.
func (s *Store) Usage(ctx context.Context, workspaceID, from, to string) ([]UsageLine, error) {
	rows, err := s.pool.Query(ctx, `SELECT month, environment, sum(count)::bigint FROM usage_counters
		WHERE workspace_id = $1 AND month >= $2 AND month <= $3
		GROUP BY month, environment ORDER BY month DESC, environment DESC`, workspaceID, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (UsageLine, error) {
		var u UsageLine
		var env string
		err := r.Scan(&u.Month, &env, &u.Signatures)
		u.Environment = model.Environment(env)
		return u, err
	})
}
