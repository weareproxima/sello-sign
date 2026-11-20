// Package store is the API's Postgres: the domain tables in the public
// schema (migrations/), never better-auth's in `auth`. Every query of the
// API lives here.
package store

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
)

var (
	// ErrNotFound: no such row, or not within the caller's reach.
	ErrNotFound = errors.New("store: not found")
	// ErrAmbiguous: a tax id names more than one organization within reach.
	ErrAmbiguous = errors.New("store: ambiguous organization")
	// ErrSelloLinked: another organization or space already names that
	// organization of Sello's IdP.
	ErrSelloLinked = errors.New("store: sello workspace already linked")
	// ErrSelloMismatch: the organization a Sello link names has another tax
	// id or country than the one asked for.
	ErrSelloMismatch = errors.New("store: linked organization has another tax id or country")
	// ErrHasDelegations: the organization is delegated to some space.
	ErrHasDelegations = errors.New("store: organization has delegations")
	// ErrDelegationExists: the space already has a delegation of it.
	ErrDelegationExists = errors.New("store: delegation exists")
	// ErrDelegationLocked: the owner can't remove a locked delegation.
	ErrDelegationLocked = errors.New("store: delegation locked")
	// ErrNotClaimable: an organization asked for can't be claimed.
	ErrNotClaimable = errors.New("store: organization not claimable")
	// ErrOrganizationExists: the space already has an organization with
	// that country and tax id.
	ErrOrganizationExists = errors.New("store: organization exists")
)

// Store is the API's database.
type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// Open connects to url (DATABASE_URL) and checks it answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("store: DATABASE_URL isn't a Postgres URL")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// New wraps a pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

// Pool is the store's pool.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping checks the database answers.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func uniqueViolation(err error) (constraint string, ok bool) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return pg.ConstraintName, true
	}
	return "", false
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Page is a request for one page of a list: Limit items after Cursor.
type Page struct {
	Limit  int
	Cursor string
}

// DefaultLimit and MaxLimit bound a page.
const (
	DefaultLimit = 50
	MaxLimit     = 100
)

// EncodeCursor makes the cursor that follows the item id: opaque to the
// client, the id in base64url.
func EncodeCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }

// DecodeCursor reads a cursor; ok is false if it isn't one EncodeCursor
// wrote.
func DecodeCursor(c string) (id string, ok bool) {
	if c == "" {
		return "", true
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", false
	}
	return ids.Canonical(string(b))
}

// cursorArg is the cursor's id for a query, NULL for the first page.
func cursorArg(p Page) any {
	id, _ := DecodeCursor(p.Cursor)
	if id == "" {
		return nil
	}
	return id
}

// limitArg is the page's size plus one, to know whether there is a next.
func limitArg(p Page) int {
	l := p.Limit
	if l <= 0 {
		l = DefaultLimit
	}
	return min(l, MaxLimit) + 1
}

// trim cuts the extra item limitArg asked for and returns the next cursor.
func trim[T any](items []T, p Page, id func(T) string) ([]T, *string) {
	l := limitArg(p) - 1
	if len(items) <= l {
		return items, nil
	}
	items = items[:l]
	c := EncodeCursor(id(items[l-1]))
	return items, &c
}

// Limits are a plan's; nil is none.
type Limits struct {
	Organizations     *int `json:"organizations"`
	ProdSignatures    *int `json:"prod_signatures"`
	TestSignatures    *int `json:"test_signatures"`
	Members           *int `json:"members"`
	RequestsPerMinute *int `json:"requests_per_minute"`
	RetentionDays     *int `json:"retention_days"`
	ProdHardCap       *int `json:"prod_hard_cap"`
	TestHardCap       *int `json:"test_hard_cap"`
}

// Workspace is a space.
type Workspace struct {
	ID               string
	Name             string
	DefaultCountry   *string
	SelloWorkspaceID *string
	Plan             *string
	Limits           Limits
	BillingVersion   int64
	ClosedAt         *time.Time
	CreatedAt        time.Time
}

// Closed says Sello deleted the space's organization.
func (w Workspace) Closed() bool { return w.ClosedAt != nil }

// Organization is a taxpayer as a space sees it.
type Organization struct {
	ID               string
	WorkspaceID      string
	Name             string
	TaxID            string
	Country          string
	ContactEmail     *string
	SelloWorkspaceID *string
	CreatedAt        time.Time
	// Reach is how the space that asked holds it.
	Reach model.Reach
	// Delegation is the one that gives the asking space its reach, when
	// it's delegated.
	Delegation *DelegationRef
}

// DelegationRef is a delegation as the delegate sees it.
type DelegationRef struct {
	ID     string
	Scopes []model.Scope
	Locked bool
}

// Certificate is what's shown of a certificate; never its material.
type Certificate struct {
	ID             string
	OrganizationID string
	Environment    model.Environment
	TaxID          *string
	Subject        string
	Issuer         string
	Serial         string
	NotBefore      *time.Time
	NotAfter       *time.Time
	Active         bool
	CreatedAt      time.Time
}

// Delegation is one as the owner sees it.
type Delegation struct {
	ID             string
	OrganizationID string
	WorkspaceID    string
	WorkspaceName  string
	Scopes         []model.Scope
	Locked         bool
	CreatedAt      time.Time
}

// APIKey is a key as it's listed.
type APIKey struct {
	ID             string
	WorkspaceID    string
	OrganizationID *string
	Environment    model.Environment
	Name           string
	Scopes         []model.Scope
	Prefix         string
	LastUsedAt     *time.Time
	CreatedAt      time.Time
}

// Member is a person in a space.
type Member struct {
	UserID *string
	Email  *string
	Name   *string
	Role   model.Role
}

// SignatureEntry is a line of the signing log.
type SignatureEntry struct {
	ID                  string
	OrganizationID      string
	OrganizationName    *string
	Environment         model.Environment
	DocumentID          string
	DocumentType        string
	APIKeyID            *string
	UserID              *string
	SignedByWorkspaceID string
	DurationMS          int
	CreatedAt           time.Time
}
