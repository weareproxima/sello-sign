package httpapi

import (
	"context"
	"time"

	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

type workspaceView struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Role             *model.Role  `json:"role"`
	Plan             *string      `json:"plan"`
	DefaultCountry   *string      `json:"default_country"`
	SelloWorkspaceID *string      `json:"sello_workspace_id"`
	Closed           bool         `json:"closed"`
	Limits           store.Limits `json:"limits"`
	// RequestsPerMinute is the signing rate the space set for itself.
	RequestsPerMinute *int      `json:"requests_per_minute"`
	CreatedAt         time.Time `json:"created_at"`
}

func workspaceOf(w store.Workspace, role model.Role) workspaceView {
	v := workspaceView{
		ID: w.ID, Name: w.Name, Plan: w.Plan, DefaultCountry: w.DefaultCountry, SelloWorkspaceID: w.SelloWorkspaceID,
		Closed: w.Closed(), Limits: w.Limits, RequestsPerMinute: w.OwnRequestsPerMinute, CreatedAt: utc(w.CreatedAt),
	}
	if role != "" {
		v.Role = &role
	}
	return v
}

type certificateView struct {
	ID          string            `json:"id"`
	Environment model.Environment `json:"environment"`
	TaxID       *string           `json:"tax_id"`
	Subject     string            `json:"subject"`
	Issuer      string            `json:"issuer"`
	Serial      string            `json:"serial"`
	NotBefore   *time.Time        `json:"not_before"`
	NotAfter    *time.Time        `json:"not_after"`
	Active      bool              `json:"active"`
	CreatedAt   time.Time         `json:"created_at"`
}

func certificateOf(c store.Certificate) certificateView {
	return certificateView{
		ID: c.ID, Environment: c.Environment, TaxID: c.TaxID, Subject: c.Subject, Issuer: c.Issuer, Serial: c.Serial,
		NotBefore: utcp(c.NotBefore), NotAfter: utcp(c.NotAfter), Active: c.Active, CreatedAt: utc(c.CreatedAt),
	}
}

type delegationRefView struct {
	ID     string        `json:"id"`
	Scopes []model.Scope `json:"scopes"`
	Locked bool          `json:"locked"`
}

type activeCertificates struct {
	Test *certificateView `json:"test"`
	Prod *certificateView `json:"prod"`
}

type organizationView struct {
	ID               string             `json:"id"`
	WorkspaceID      string             `json:"workspace_id"`
	Name             string             `json:"name"`
	TaxID            string             `json:"tax_id"`
	Country          string             `json:"country"`
	ContactEmail     *string            `json:"contact_email"`
	Relation         model.Relation     `json:"relation"`
	SelloWorkspaceID *string            `json:"sello_workspace_id"`
	Delegation       *delegationRefView `json:"delegation"`
	Certificates     activeCertificates `json:"certificates"`
	CreatedAt        time.Time          `json:"created_at"`
}

// organizationsOf renders organizations with their active certificates.
func (s *Server) organizationsOf(ctx context.Context, orgs []store.Organization) ([]organizationView, error) {
	idList := make([]string, len(orgs))
	for i, o := range orgs {
		idList[i] = o.ID
	}
	active, err := s.Store.ActiveCertificates(ctx, idList)
	if err != nil {
		return nil, err
	}
	out := make([]organizationView, len(orgs))
	for i, o := range orgs {
		v := organizationView{
			ID: o.ID, WorkspaceID: o.WorkspaceID, Name: o.Name, TaxID: o.TaxID, Country: o.Country,
			ContactEmail: o.ContactEmail, Relation: o.Reach.Relation, SelloWorkspaceID: o.SelloWorkspaceID,
			CreatedAt: utc(o.CreatedAt),
		}
		if d := o.Delegation; d != nil {
			v.Delegation = &delegationRefView{ID: d.ID, Scopes: nonNil(d.Scopes), Locked: d.Locked}
		}
		if c, ok := active[o.ID][model.Test]; ok {
			cv := certificateOf(c)
			v.Certificates.Test = &cv
		}
		if c, ok := active[o.ID][model.Prod]; ok {
			cv := certificateOf(c)
			v.Certificates.Prod = &cv
		}
		out[i] = v
	}
	return out, nil
}

func (s *Server) organizationOf(ctx context.Context, o store.Organization) (organizationView, error) {
	vs, err := s.organizationsOf(ctx, []store.Organization{o})
	if err != nil {
		return organizationView{}, err
	}
	return vs[0], nil
}

type delegationView struct {
	ID            string        `json:"id"`
	WorkspaceID   string        `json:"workspace_id"`
	WorkspaceName string        `json:"workspace_name"`
	Scopes        []model.Scope `json:"scopes"`
	Locked        bool          `json:"locked"`
	CreatedAt     time.Time     `json:"created_at"`
}

func delegationOf(d store.Delegation) delegationView {
	return delegationView{ID: d.ID, WorkspaceID: d.WorkspaceID, WorkspaceName: d.WorkspaceName, Scopes: nonNil(d.Scopes),
		Locked: d.Locked, CreatedAt: utc(d.CreatedAt)}
}

type apiKeyView struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Environment    model.Environment `json:"environment"`
	Scopes         []model.Scope     `json:"scopes"`
	OrganizationID *string           `json:"organization_id"`
	Prefix         string            `json:"prefix"`
	// RequestsPerMinute is the key's own signing rate; nil is none.
	RequestsPerMinute *int       `json:"requests_per_minute"`
	LastUsedAt        *time.Time `json:"last_used_at"`
	CreatedAt         time.Time  `json:"created_at"`
	// Key is the key itself, only when it's created.
	Key string `json:"key,omitempty"`
}

func apiKeyOf(k store.APIKey) apiKeyView {
	return apiKeyView{ID: k.ID, Name: k.Name, Environment: k.Environment, Scopes: nonNil(k.Scopes),
		OrganizationID: k.OrganizationID, Prefix: k.Prefix, RequestsPerMinute: k.RequestsPerMinute,
		LastUsedAt: utcp(k.LastUsedAt), CreatedAt: utc(k.CreatedAt)}
}

type signatureView struct {
	ID                  string            `json:"id"`
	OrganizationID      string            `json:"organization_id"`
	OrganizationName    *string           `json:"organization_name"`
	Environment         model.Environment `json:"environment"`
	DocumentID          string            `json:"document_id"`
	DocumentType        string            `json:"document_type"`
	APIKeyID            *string           `json:"api_key_id"`
	UserID              *string           `json:"user_id"`
	SignedByWorkspaceID string            `json:"signed_by_workspace_id"`
	DurationMS          int               `json:"duration_ms"`
	CreatedAt           time.Time         `json:"created_at"`
}

func signatureOf(e store.SignatureEntry) signatureView {
	return signatureView{ID: e.ID, OrganizationID: e.OrganizationID, OrganizationName: e.OrganizationName,
		Environment: e.Environment, DocumentID: e.DocumentID, DocumentType: e.DocumentType, APIKeyID: e.APIKeyID,
		UserID: e.UserID, SignedByWorkspaceID: e.SignedByWorkspaceID, DurationMS: e.DurationMS, CreatedAt: utc(e.CreatedAt)}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
