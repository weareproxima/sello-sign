package store

import (
	"context"

	"github.com/jackc/pgx/v5"

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

// InsertCertificate stores a certificate whose id the caller chose (the
// material is sealed for it), and makes it the active one of its
// environment if activate.
func (s *Store) InsertCertificate(ctx context.Context, c Certificate, sealed []byte, activate bool) (Certificate, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if activate {
			if _, err := tx.Exec(ctx, `UPDATE certificates SET active = false
				WHERE organization_id = $1 AND environment = $2 AND active`, c.OrganizationID, string(c.Environment)); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO certificates
			(id, organization_id, environment, material, tax_id, subject, issuer, serial, not_before, not_after, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			c.ID, c.OrganizationID, string(c.Environment), sealed, c.TaxID, c.Subject, c.Issuer, c.Serial,
			c.NotBefore, c.NotAfter, activate)
		return err
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
// active one of its environment) or deactivates it.
func (s *Store) SetCertificateActive(ctx context.Context, organizationID, id string, active bool) (Certificate, error) {
	if !ids.Valid(id) {
		return Certificate{}, ErrNotFound
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var env string
		if err := tx.QueryRow(ctx, `SELECT environment FROM certificates WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
			organizationID, id).Scan(&env); err != nil {
			return notFound(err)
		}
		if active {
			if _, err := tx.Exec(ctx, `UPDATE certificates SET active = false
				WHERE organization_id = $1 AND environment = $2 AND active AND id <> $3`, organizationID, env, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE certificates SET active = $3 WHERE organization_id = $1 AND id = $2`, organizationID, id, active)
		return err
	})
	if err != nil {
		return Certificate{}, err
	}
	return s.Certificate(ctx, organizationID, id)
}

// DeleteCertificate deletes one of an organization's certificates.
func (s *Store) DeleteCertificate(ctx context.Context, organizationID, id string) error {
	if !ids.Valid(id) {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM certificates WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ActiveMaterial is the active certificate of an organization in env with
// its sealed material.
func (s *Store) ActiveMaterial(ctx context.Context, organizationID string, env model.Environment) (Certificate, []byte, error) {
	var sealed []byte
	c, err := scanCertificate(s.pool.QueryRow(ctx, `SELECT `+certificateColumns+`, c.material FROM certificates c
		WHERE c.organization_id = $1 AND c.environment = $2 AND c.active`, organizationID, string(env)), &sealed)
	return c, sealed, err
}
