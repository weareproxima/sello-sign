package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

const keyColumns = `k.id, k.workspace_id, k.organization_id, k.environment, k.name, k.scopes, k.prefix, k.last_used_at, k.created_at`

func scanKey(row pgx.Row) (APIKey, error) {
	var k APIKey
	var env string
	var scopes []string
	if err := row.Scan(&k.ID, &k.WorkspaceID, &k.OrganizationID, &env, &k.Name, &scopes, &k.Prefix, &k.LastUsedAt, &k.CreatedAt); err != nil {
		return APIKey{}, notFound(err)
	}
	k.Environment = model.Environment(env)
	k.Scopes = model.ScopesOf(scopes)
	return k, nil
}

// NewAPIKey is what creates a key; Hash and Prefix come from auth.NewKey.
type NewAPIKey struct {
	WorkspaceID    string
	OrganizationID *string
	Environment    model.Environment
	Name           string
	Scopes         []model.Scope
	Hash           []byte
	Prefix         string
	CreatedBy      string
}

// CreateAPIKey stores a key.
func (s *Store) CreateAPIKey(ctx context.Context, n NewAPIKey) (APIKey, error) {
	id := ids.New()
	var by any
	if n.CreatedBy != "" {
		by = n.CreatedBy
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO api_keys
		(id, workspace_id, organization_id, environment, name, scopes, hash, prefix, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, n.WorkspaceID, n.OrganizationID, string(n.Environment), n.Name, model.Strings(n.Scopes), n.Hash, n.Prefix, by); err != nil {
		return APIKey{}, err
	}
	return scanKey(s.pool.QueryRow(ctx, `SELECT `+keyColumns+` FROM api_keys k WHERE k.id = $1`, id))
}

// APIKeys lists a space's live keys, newest first.
func (s *Store) APIKeys(ctx context.Context, workspaceID, only string, p Page) ([]APIKey, *string, error) {
	var onlyArg any
	if only != "" {
		onlyArg = only
	}
	rows, err := s.pool.Query(ctx, `SELECT `+keyColumns+` FROM api_keys k
		WHERE k.workspace_id = $1 AND k.revoked_at IS NULL
		AND ($2::uuid IS NULL OR k.organization_id = $2)
		AND ($3::uuid IS NULL OR k.id < $3)
		ORDER BY k.id DESC LIMIT $4`, workspaceID, onlyArg, cursorArg(p), limitArg(p))
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (APIKey, error) { return scanKey(r) })
	if err != nil {
		return nil, nil, err
	}
	items, next := trim(items, p, func(k APIKey) string { return k.ID })
	return items, next, nil
}

// APIKey reads one of a space's live keys.
func (s *Store) APIKey(ctx context.Context, workspaceID, id string) (APIKey, error) {
	if !ids.Valid(id) {
		return APIKey{}, ErrNotFound
	}
	return scanKey(s.pool.QueryRow(ctx, `SELECT `+keyColumns+` FROM api_keys k
		WHERE k.workspace_id = $1 AND k.id = $2 AND k.revoked_at IS NULL`, workspaceID, id))
}

// RevokeAPIKey revokes a key and returns its hash, to drop it from the
// cache.
func (s *Store) RevokeAPIKey(ctx context.Context, workspaceID, id string) ([]byte, error) {
	if !ids.Valid(id) {
		return nil, ErrNotFound
	}
	var hash []byte
	err := s.pool.QueryRow(ctx, `UPDATE api_keys SET revoked_at = now()
		WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL RETURNING hash`, workspaceID, id).Scan(&hash)
	return hash, notFound(err)
}

// KeyByHash finds a live key by its hash.
func (s *Store) KeyByHash(ctx context.Context, hash []byte) (auth.Key, error) {
	var k auth.Key
	var org *string
	var env string
	var scopes []string
	err := s.pool.QueryRow(ctx, `SELECT id, workspace_id, organization_id, environment, scopes
		FROM api_keys WHERE hash = $1 AND revoked_at IS NULL`, hash).Scan(&k.ID, &k.WorkspaceID, &org, &env, &scopes)
	if err != nil {
		return auth.Key{}, notFound(err)
	}
	if org != nil {
		k.OrganizationID = *org
	}
	k.Environment = model.Environment(env)
	k.Scopes = model.ScopesOf(scopes)
	return k, nil
}

// TouchAPIKey records that a key was used now.
func (s *Store) TouchAPIKey(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	return err
}
