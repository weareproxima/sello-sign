package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

var (
	// ErrOrganizationLimit: the space already owns as many organizations as
	// its plan allows.
	ErrOrganizationLimit = errors.New("store: organization limit reached")
	// ErrHasPlan: the space's limits are its plan's, which billing pushes.
	ErrHasPlan = errors.New("store: the space has a plan")
)

// MonthUsage is what a space signed in env in month ("YYYY-MM"): the sum
// of its counters, kept in one row (workspace_usage).
func (s *Store) MonthUsage(ctx context.Context, workspaceID string, env model.Environment, month string) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count FROM workspace_usage WHERE workspace_id = $1 AND environment = $2 AND month = $3`,
		workspaceID, string(env), month).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// LimitsPatch is what the console sets of a space without a plan; nil
// leaves a limit, and a nil *int clears it.
type LimitsPatch struct {
	Organizations     **int
	ProdSignatures    **int
	TestSignatures    **int
	Members           **int
	RequestsPerMinute **int
	RetentionDays     **int
}

// SetLimits sets the limits of a space without a plan (a self-hosted
// install's, or one made with the console). A space of the cloud's, with
// a plan, is ErrHasPlan: billing pushes its limits.
func (s *Store) SetLimits(ctx context.Context, id string, p LimitsPatch) (Workspace, error) {
	if !ids.Valid(id) {
		return Workspace{}, ErrNotFound
	}
	args := []any{id}
	set := ""
	for _, f := range []struct {
		column string
		value  **int
	}{
		{"max_organizations", p.Organizations}, {"max_prod_signatures", p.ProdSignatures},
		{"max_test_signatures", p.TestSignatures}, {"max_members", p.Members},
		{"requests_per_minute", p.RequestsPerMinute}, {"retention_days", p.RetentionDays},
	} {
		if f.value == nil {
			continue
		}
		args = append(args, *f.value)
		set += f.column + " = $" + strconv.Itoa(len(args)) + ", "
	}
	tag, err := s.pool.Exec(ctx, `UPDATE workspaces SET `+set+`updated_at = now()
		WHERE id = $1 AND plan IS NULL`, args...)
	if err != nil {
		return Workspace{}, err
	}
	if tag.RowsAffected() == 0 {
		w, err := s.Workspace(ctx, id)
		if err != nil {
			return Workspace{}, err
		}
		if w.Plan != nil {
			return Workspace{}, ErrHasPlan
		}
	}
	return s.Workspace(ctx, id)
}

// RetentionBatch is how many log entries one statement deletes, so that
// purging a large backlog never holds a long lock.
const RetentionBatch = 5000

// PurgeExpiredSignatures deletes, from the signing log, what each space
// signed longer ago than its retention (Limits.RetentionDays: its plan's
// or, without a plan, the install's; nil keeps it). An entry follows the
// retention of the space that signed it, whose log it is and where it was
// counted, also when the organization is another space's. It returns how
// many entries it deleted.
func (s *Store) PurgeExpiredSignatures(ctx context.Context, now time.Time) (int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workspaceColumns+` FROM workspaces w`)
	if err != nil {
		return 0, err
	}
	var spaces []Workspace
	for rows.Next() {
		w, err := s.scanWorkspace(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		if w.Limits.RetentionDays != nil {
			spaces = append(spaces, w)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var total int64
	for _, w := range spaces {
		before := ids.Floor(now.AddDate(0, 0, -*w.Limits.RetentionDays))
		for {
			// The ids are UUIDv7: every entry signed before the cutoff has
			// an id below its floor, and signing_log_signer reaches them.
			tag, err := s.pool.Exec(ctx, `DELETE FROM signing_log WHERE id IN (
					SELECT id FROM signing_log WHERE signed_by_workspace_id = $1 AND id < $2
					ORDER BY id LIMIT $3)`, w.ID, before, RetentionBatch)
			if err != nil {
				return total, err
			}
			total += tag.RowsAffected()
			if tag.RowsAffected() < RetentionBatch || ctx.Err() != nil {
				break
			}
		}
	}
	return total, ctx.Err()
}
