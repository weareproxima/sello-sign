package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/ids"
)

// AuditEvent is an entry of a space's audit log.
type AuditEvent struct {
	ID          string
	WorkspaceID string
	Actor       audit.Actor
	Action      audit.Action
	TargetType  audit.TargetType
	TargetID    *string
	TargetLabel string
	// Details are the event's JSON object.
	Details   json.RawMessage
	CreatedAt time.Time
}

// event is what a change records in a space's audit log.
type event struct {
	workspace string
	action    audit.Action
	target    audit.TargetType
	targetID  string
	label     string
	details   map[string]any
	// actor, if set, acted instead of the context's: billing, or another
	// space.
	actor *audit.Actor
}

// record writes e in tx, the transaction of the change it records, so that
// the event exists exactly when the change does. The actor is the
// context's (audit.ActorFrom), unless e names one; a key's and a space's
// names are read here, as they are at this moment.
func (s *Store) record(ctx context.Context, tx pgx.Tx, e event) error {
	a := audit.ActorFrom(ctx)
	if e.actor != nil {
		a = *e.actor
	}
	name := a.Name
	var lookup string
	switch a.Type {
	case audit.ActorAPIKey:
		lookup = `SELECT name FROM api_keys WHERE id = $1`
	case audit.ActorWorkspace:
		lookup = `SELECT name FROM workspaces WHERE id = $1`
	}
	if lookup != "" && ids.Valid(a.ID) {
		if err := tx.QueryRow(ctx, lookup, a.ID).Scan(&name); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	details := e.details
	if details == nil {
		details = map[string]any{}
	}
	body, err := json.Marshal(details)
	if err != nil {
		return err
	}
	now := s.now()
	_, err = tx.Exec(ctx, `INSERT INTO audit_events
		(id, workspace_id, actor_type, actor_id, actor_email, actor_name, via, action, target_type, target_id, target_label, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		ids.NewAt(now).String(), e.workspace, string(a.Type), nullable(a.ID), nullable(a.Email), name, nullable(a.Via),
		string(e.action), string(e.target), nullable(e.targetID), e.label, body, now)
	return err
}

// recordOnOrganization records e, a change to an organization made from the
// space acting, in that space and, if another space owns the organization
// (a delegation that lets acting change its certificates), also in the
// owner's, where the actor is the acting space: its person or its key are
// none of the owner's business.
func (s *Store) recordOnOrganization(ctx context.Context, tx pgx.Tx, acting, owner string, e event) error {
	if acting == "" {
		acting = owner
	}
	e.workspace = acting
	if err := s.record(ctx, tx, e); err != nil {
		return err
	}
	if owner == acting {
		return nil
	}
	by := audit.Space(acting)
	e.workspace, e.actor = owner, &by
	return s.record(ctx, tx, e)
}

// RecordEvent records an event that isn't a change the store makes (an
// export of the space's data), on its own.
func (s *Store) RecordEvent(ctx context.Context, workspaceID string, action audit.Action, target audit.TargetType, targetID, label string, details map[string]any) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.record(ctx, tx, event{workspace: workspaceID, action: action, target: target, targetID: targetID, label: label, details: details})
	})
}

// memberLabel is how an event names a person of a space: their e-mail, or
// their name, as the API last knew them; "" if it never did.
func memberLabel(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var email, name string
	err := tx.QueryRow(ctx, `SELECT email, name FROM users WHERE id = $1`, userID).Scan(&email, &name)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil
	case err != nil:
		return "", err
	case email == "":
		return name, nil
	}
	return email, nil
}

// workspaceName is a space's name, "" if it isn't there.
func workspaceName(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM workspaces WHERE id = $1`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// AuditFilter narrows a space's audit log; a zero field doesn't.
type AuditFilter struct {
	Action     audit.Action
	TargetType audit.TargetType
	TargetID   string
	ActorType  audit.ActorType
	ActorID    string
	From, To   *time.Time
}

const auditColumns = `e.id, e.workspace_id, e.actor_type, e.actor_id, e.actor_email, e.actor_name, e.via,
	e.action, e.target_type, e.target_id, e.target_label, e.details, e.created_at`

func scanAuditEvent(row pgx.Row) (AuditEvent, error) {
	var e AuditEvent
	var actorType, action, target string
	var actorID, email, via *string
	if err := row.Scan(&e.ID, &e.WorkspaceID, &actorType, &actorID, &email, &e.Actor.Name, &via,
		&action, &target, &e.TargetID, &e.TargetLabel, &e.Details, &e.CreatedAt); err != nil {
		return AuditEvent{}, notFound(err)
	}
	e.Actor.Type, e.Action, e.TargetType = audit.ActorType(actorType), audit.Action(action), audit.TargetType(target)
	if actorID != nil {
		e.Actor.ID = *actorID
	}
	if email != nil {
		e.Actor.Email = *email
	}
	if via != nil {
		e.Actor.Via = *via
	}
	return e, nil
}

// AuditEvents lists a space's audit log, newest first.
func (s *Store) AuditEvents(ctx context.Context, workspaceID string, f AuditFilter, p Page) ([]AuditEvent, *string, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+auditColumns+` FROM audit_events e
		WHERE e.workspace_id = $1
		AND ($2 = '' OR e.action = $2)
		AND ($3 = '' OR e.target_type = $3)
		AND ($4::text IS NULL OR e.target_id = $4)
		AND ($5 = '' OR e.actor_type = $5)
		AND ($6::text IS NULL OR e.actor_id = $6)
		AND ($7::timestamptz IS NULL OR e.created_at >= $7)
		AND ($8::timestamptz IS NULL OR e.created_at < $8)
		AND ($9::uuid IS NULL OR e.id < $9)
		ORDER BY e.id DESC LIMIT $10`,
		workspaceID, string(f.Action), string(f.TargetType), nullable(f.TargetID), string(f.ActorType), nullable(f.ActorID),
		f.From, f.To, cursorArg(p), limitArg(p))
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AuditEvent, error) { return scanAuditEvent(r) })
	if err != nil {
		return nil, nil, err
	}
	items, next := trim(items, p, func(e AuditEvent) string { return e.ID })
	return items, next, nil
}

// EachAuditEvent calls fn with every event of a space's audit log, newest
// first, reading it in batches so that no connection is held while fn
// writes (an export).
func (s *Store) EachAuditEvent(ctx context.Context, workspaceID string, fn func(AuditEvent) error) error {
	var cursor any
	for {
		rows, err := s.pool.Query(ctx, `SELECT `+auditColumns+` FROM audit_events e
			WHERE e.workspace_id = $1 AND ($2::uuid IS NULL OR e.id < $2) ORDER BY e.id DESC LIMIT $3`,
			workspaceID, cursor, exportBatch)
		if err != nil {
			return err
		}
		batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AuditEvent, error) { return scanAuditEvent(r) })
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

// PurgeExpiredAuditEvents deletes the events older than
// audit.RetentionDays, whatever the space's plan, in batches. It returns
// how many it deleted.
func (s *Store) PurgeExpiredAuditEvents(ctx context.Context, now time.Time) (int64, error) {
	// The ids are UUIDv7: every event made before the cutoff has an id
	// below its floor, and the primary key reaches them.
	before := ids.Floor(now.Add(-audit.Retention))
	var total int64
	for {
		tag, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE id IN (
				SELECT id FROM audit_events WHERE id < $1 ORDER BY id LIMIT $2)`, before, RetentionBatch)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < RetentionBatch || ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}
