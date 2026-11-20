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

const certificateColumns = `c.id, c.organization_id, c.environment, c.tax_id, c.subject, c.issuer, c.serial,
	c.not_before, c.not_after, c.active, c.created_at`

func scanCertificate(row pgx.Row, extra ...any) (Certificate, error) {
	var c Certificate
	var env string
	dest := append([]any{&c.ID, &c.OrganizationID, &env, &c.TaxID, &c.Subject, &c.Issuer, &c.Serial,
		&c.NotBefore, &c.NotAfter, &c.Active, &c.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Certificate{}, notFound(err)
	}
	c.Environment = model.Environment(env)
	return c, nil
}

// Certificates lists an organization's certificates, newest first.
func (s *Store) Certificates(ctx context.Context, organizationID string) ([]Certificate, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+certificateColumns+` FROM certificates c
		WHERE c.organization_id = $1 ORDER BY c.id DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Certificate, error) { return scanCertificate(r) })
}

// ActiveCertificates are the active certificate of each environment of
// each organization: organization id → environment → certificate.
func (s *Store) ActiveCertificates(ctx context.Context, organizationIDs []string) (map[string]map[model.Environment]Certificate, error) {
	out := map[string]map[model.Environment]Certificate{}
	if len(organizationIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+certificateColumns+` FROM certificates c
		WHERE c.organization_id = ANY($1::uuid[]) AND c.active`, organizationIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		if out[c.OrganizationID] == nil {
			out[c.OrganizationID] = map[model.Environment]Certificate{}
		}
		out[c.OrganizationID][c.Environment] = c
	}
	return out, rows.Err()
}

// organizationOwner is the space that owns an organization and its name.
func organizationOwner(ctx context.Context, tx pgx.Tx, organizationID string) (owner, name string, err error) {
	err = tx.QueryRow(ctx, `SELECT workspace_id, name FROM organizations WHERE id = $1`, organizationID).Scan(&owner, &name)
	return owner, name, notFound(err)
}

// certificateDetails are what an event of a certificate says of it: never
// its material.
func certificateDetails(organizationID string, env model.Environment, serial string, notAfter *time.Time) map[string]any {
	d := map[string]any{"organization_id": organizationID, "environment": env, "serial": serial}
	if notAfter != nil {
		d["not_after"] = notAfter.UTC()
	}
	return d
}

// deactivateActive deactivates the active certificate of an organization in
// env, but keep, and returns its id ("" if there was none).
func deactivateActive(ctx context.Context, tx pgx.Tx, organizationID string, env model.Environment, keep string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `UPDATE certificates SET active = false
		WHERE organization_id = $1 AND environment = $2 AND active AND id::text <> $3 RETURNING id::text`,
		organizationID, string(env), keep).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// InsertCertificate stores a certificate whose id the caller chose (the
// material is sealed for it), and makes it the active one of its
// environment if activate. workspaceID is the space that uploads it: the
// owner's, or one the organization is delegated to.
func (s *Store) InsertCertificate(ctx context.Context, workspaceID string, c Certificate, sealed []byte, activate bool) (Certificate, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		owner, name, err := organizationOwner(ctx, tx, c.OrganizationID)
		if err != nil {
			return err
		}
		replaced := ""
		if activate {
			if replaced, err = deactivateActive(ctx, tx, c.OrganizationID, c.Environment, c.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO certificates
			(id, organization_id, environment, material, tax_id, subject, issuer, serial, not_before, not_after, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			c.ID, c.OrganizationID, string(c.Environment), sealed, c.TaxID, c.Subject, c.Issuer, c.Serial,
			c.NotBefore, c.NotAfter, activate); err != nil {
			return err
		}
		details := certificateDetails(c.OrganizationID, c.Environment, c.Serial, c.NotAfter)
		details["active"] = activate
		if replaced != "" {
			details["replaced_certificate_id"] = replaced
		}
		return s.recordOnOrganization(ctx, tx, workspaceID, owner, event{action: audit.CertificateUploaded,
			target: audit.TargetCertificate, targetID: c.ID, label: name, details: details})
	})
	if err != nil {
		return Certificate{}, err
	}
	return s.Certificate(ctx, c.OrganizationID, c.ID)
}

// Certificate reads one of an organization's certificates.
func (s *Store) Certificate(ctx context.Context, organizationID, id string) (Certificate, error) {
	if !ids.Valid(id) {
		return Certificate{}, ErrNotFound
	}
	return scanCertificate(s.pool.QueryRow(ctx, `SELECT `+certificateColumns+` FROM certificates c
		WHERE c.organization_id = $1 AND c.id = $2`, organizationID, id))
}

// SetCertificateActive activates a certificate (deactivating the other
// active one of its environment) or deactivates it, from the space
// workspaceID (the owner's, or a delegate's). Setting what it already is
// changes nothing, and records nothing.
func (s *Store) SetCertificateActive(ctx context.Context, workspaceID, organizationID, id string, active bool) (Certificate, error) {
	if !ids.Valid(id) {
		return Certificate{}, ErrNotFound
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var env, serial string
		var was bool
		var notAfter *time.Time
		if err := tx.QueryRow(ctx, `SELECT environment, serial, active, not_after FROM certificates
			WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
			organizationID, id).Scan(&env, &serial, &was, &notAfter); err != nil {
			return notFound(err)
		}
		if was == active {
			return nil
		}
		owner, name, err := organizationOwner(ctx, tx, organizationID)
		if err != nil {
			return err
		}
		replaced := ""
		if active {
			if replaced, err = deactivateActive(ctx, tx, organizationID, model.Environment(env), id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE certificates SET active = $3 WHERE organization_id = $1 AND id = $2`,
			organizationID, id, active); err != nil {
			return err
		}
		e := event{action: audit.CertificateDeactivated, target: audit.TargetCertificate, targetID: id, label: name,
			details: certificateDetails(organizationID, model.Environment(env), serial, notAfter)}
		if active {
			e.action = audit.CertificateActivated
		}
		if replaced != "" {
			e.details["replaced_certificate_id"] = replaced
		}
		return s.recordOnOrganization(ctx, tx, workspaceID, owner, e)
	})
	if err != nil {
		return Certificate{}, err
	}
	return s.Certificate(ctx, organizationID, id)
}

// DeleteCertificate deletes one of an organization's certificates, from
// the space workspaceID (the owner's, or a delegate's).
func (s *Store) DeleteCertificate(ctx context.Context, workspaceID, organizationID, id string) error {
	if !ids.Valid(id) {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var env, serial string
		var active bool
		var notAfter *time.Time
		if err := tx.QueryRow(ctx, `DELETE FROM certificates WHERE organization_id = $1 AND id = $2
			RETURNING environment, serial, active, not_after`, organizationID, id).Scan(&env, &serial, &active, &notAfter); err != nil {
			return notFound(err)
		}
		owner, name, err := organizationOwner(ctx, tx, organizationID)
		if err != nil {
			return err
		}
		details := certificateDetails(organizationID, model.Environment(env), serial, notAfter)
		details["active"] = active
		return s.recordOnOrganization(ctx, tx, workspaceID, owner, event{action: audit.CertificateDeleted,
			target: audit.TargetCertificate, targetID: id, label: name, details: details})
	})
}

// ActiveMaterial is the active certificate of an organization in env with
// its sealed material.
func (s *Store) ActiveMaterial(ctx context.Context, organizationID string, env model.Environment) (Certificate, []byte, error) {
	var sealed []byte
	c, err := scanCertificate(s.pool.QueryRow(ctx, `SELECT `+certificateColumns+`, c.material FROM certificates c
		WHERE c.organization_id = $1 AND c.environment = $2 AND c.active`, organizationID, string(env)), &sealed)
	return c, sealed, err
}
