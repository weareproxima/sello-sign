package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/billing"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

func canonicalOr(s string) string {
	id, _ := ids.Canonical(s)
	return id
}

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// me handles GET /me: the person and their spaces. It also keeps who they
// are, to name them among the members.
func (s *Server) me(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	ctx := r.Context()
	if err := s.Store.UpsertUser(ctx, u.Subject, u.Email, u.Name, u.SelloSub); err != nil {
		internalError(w, r, err)
		return
	}
	spaces, err := s.Store.WorkspacesOf(ctx, u.Subject, u.SelloSub)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       userView{ID: u.Subject, Email: u.Email, Name: u.Name},
		"workspaces": nonNil(mapSlice(spaces, func(ws store.WorkspaceWithRole) workspaceView { return workspaceOf(ws.Workspace, ws.Role) })),
	})
}

type selloOrganizationView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Country     string     `json:"country"`
	Role        model.Role `json:"role"`
	Status      string     `json:"status"`
	WorkspaceID *string    `json:"workspace_id"`
	MaxMembers  *int       `json:"max_members"`
}

// selloOrganizations handles GET /me/sello-organizations (cloud): the
// person's organizations at Sello's IdP and what they can do with each
// here, as organizationOptions of sign-old.
func (s *Server) selloOrganizations(w http.ResponseWriter, r *http.Request, c *call) {
	if !s.Cloud {
		writeProblem(w, errNotCloud)
		return
	}
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	ctx := r.Context()
	selloIDs := make([]string, len(u.SelloWorkspaces))
	for i, o := range u.SelloWorkspaces {
		selloIDs[i] = o.ID
	}
	spaces, err := s.Store.WorkspacesBySelloIDs(ctx, selloIDs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	mine, err := s.Store.WorkspacesOf(ctx, u.Subject, u.SelloSub)
	if err != nil {
		internalError(w, r, err)
		return
	}
	member := map[string]bool{}
	for _, m := range mine {
		member[m.ID] = true
	}
	out := make([]selloOrganizationView, 0, len(u.SelloWorkspaces))
	for _, o := range u.SelloWorkspaces {
		role, ok := model.ParseRole(o.Role)
		if !ok {
			role = model.Member
		}
		v := selloOrganizationView{ID: o.ID, Name: o.Name, Country: o.Country, Role: role}
		space, has := spaces[o.ID]
		switch {
		case !has && role == model.Member:
			v.Status = "ask"
		case !has:
			v.Status = "available"
		case space.Closed():
			v.Status = "closed"
		case member[space.ID]:
			v.Status = "open"
			v.WorkspaceID = &space.ID
		default:
			v.Status = "full"
			v.MaxMembers = space.Limits.Members
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, listOf(out, nil))
}

// createWorkspace handles POST /workspaces: a self-hosted space with its
// creator as owner, or, in the cloud, the space of an organization of
// Sello's IdP (openSpace of sign-old).
func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	var body struct {
		Name             *string `json:"name"`
		DefaultCountry   *string `json:"default_country"`
		SelloWorkspaceID *string `json:"sello_workspace_id"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	ctx := r.Context()
	if err := s.Store.UpsertUser(ctx, u.Subject, u.Email, u.Name, u.SelloSub); err != nil {
		internalError(w, r, err)
		return
	}
	if s.Cloud {
		if body.SelloWorkspaceID == nil || *body.SelloWorkspaceID == "" || body.Name != nil {
			writeProblem(w, invalid("En la nube, un espacio se abre con «sello_workspace_id», la organización de Sello que lo paga."))
			return
		}
		s.openSpace(w, r, u, *body.SelloWorkspaceID)
		return
	}
	if body.SelloWorkspaceID != nil {
		writeProblem(w, invalid("«sello_workspace_id» solo existe en la nube."))
		return
	}
	name := trimmed(body.Name)
	if name == nil || len(*name) > 120 {
		writeProblem(w, invalid("«name» es obligatorio, de hasta 120 caracteres."))
		return
	}
	country, p := s.countryOf(body.DefaultCountry)
	if p != nil {
		writeProblem(w, p)
		return
	}
	ws, err := s.Store.CreateWorkspace(ctx, *name, country, u.Subject)
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(ctx, "workspace created", "workspace_id", ws.ID)
	writeJSON(w, http.StatusCreated, workspaceOf(ws, model.Owner))
}

// countryOf validates an optional country code.
func (s *Server) countryOf(in *string) (*string, *Problem) {
	if in == nil {
		return nil, nil
	}
	c := strings.ToUpper(strings.TrimSpace(*in))
	if c == "" {
		return nil, nil
	}
	if !model.ValidCountry(c) {
		return nil, invalid("El país debe ser un código ISO 3166-1 alfa-2, como SV.")
	}
	return &c, nil
}

// openSpace opens the space of the organization selloID of the IdP for
// the person, creating it the first time: billing activates it and
// answers its plan, its limits, its name, its country and its members.
// Only its owner or an admin creates it; anyone in the organization opens
// one that exists, as far as its plan admits members.
func (s *Server) openSpace(w http.ResponseWriter, r *http.Request, u *auth.User, selloID string) {
	ctx := r.Context()
	var org *auth.SelloWorkspace
	for i := range u.SelloWorkspaces {
		if u.SelloWorkspaces[i].ID == selloID {
			org = &u.SelloWorkspaces[i]
		}
	}
	if org == nil || u.SelloSub == "" {
		writeProblem(w, forbidden("Esa no es una de sus organizaciones en Sello."))
		return
	}
	role, _ := model.ParseRole(org.Role)
	_, err := s.Store.WorkspaceBySelloID(ctx, selloID)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, r, err)
		return
	}
	if !exists && !role.Manages() {
		writeProblem(w, problem(http.StatusForbidden, "ask-admin", "Solo quien administra la organización en Sello puede empezar a usar Sello Sign con ella."))
		return
	}
	if s.Billing == nil {
		writeProblem(w, problem(http.StatusServiceUnavailable, "billing-unavailable", "No se pudo consultar el plan de la organización. Vuelva a intentarlo."))
		return
	}
	st, err := s.Billing.Activate(ctx, selloID)
	switch {
	case errors.Is(err, billing.ErrUnknown):
		writeProblem(w, problem(http.StatusConflict, "sello-organization-unknown", "Sello todavía no termina de registrar la organización. Vuelva a intentarlo en un momento."))
		return
	case errors.Is(err, billing.ErrClosed):
		writeProblem(w, problem(http.StatusGone, "sello-organization-closed", "La organización se borró en Sello."))
		return
	case err != nil:
		writeProblem(w, problem(http.StatusServiceUnavailable, "billing-unavailable", "No se pudo consultar el plan de la organización. Vuelva a intentarlo."))
		return
	}
	id, created, err := s.Store.CreateCloudWorkspace(ctx, st, org.Name)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// The state answered may be newer than what was pushed: it's applied
	// the same way, members included.
	res, err := s.Store.ApplyState(ctx, st, s.PlatformWorkspace)
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.dropKeys(ctx, res)
	myRole, err := s.Store.RoleIn(ctx, id, u.Subject, u.SelloSub)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if myRole == "" {
		writeProblem(w, problem(http.StatusConflict, "workspace-full", "El plan de la organización no admite más miembros en Sello Sign."))
		return
	}
	ws, err := s.Store.Workspace(ctx, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(ctx, "space opened", "workspace_id", id, "created", created)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, workspaceOf(ws, myRole))
}

// getWorkspace handles GET /workspaces/{w}.
func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request, c *call) {
	writeJSON(w, http.StatusOK, workspaceOf(c.ws, c.role))
}

// updateWorkspace handles PATCH /workspaces/{w}: owners and admins.
func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request, c *call) {
	if c.user() == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio puede cambiarlo."))
		return
	}
	var body struct {
		Name              *string        `json:"name"`
		DefaultCountry    optionalString `json:"default_country"`
		RequestsPerMinute optionalInt    `json:"requests_per_minute"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	var patch store.WorkspacePatch
	if body.Name != nil {
		name := trimmed(body.Name)
		if name == nil || len(*name) > 120 {
			writeProblem(w, invalid("«name» no puede quedar vacío y tiene hasta 120 caracteres."))
			return
		}
		patch.Name = name
	}
	if body.DefaultCountry.Set {
		country, p := s.countryOf(body.DefaultCountry.Value)
		if p != nil {
			writeProblem(w, p)
			return
		}
		patch.DefaultCountry = &country
	}
	if body.RequestsPerMinute.Set {
		rate := body.RequestsPerMinute.Value
		if rate != nil && (*rate < 1 || *rate > maxRequestsPerMinute) {
			writeProblem(w, invalid("«requests_per_minute» es un número de 1 a 1000000, o null."))
			return
		}
		if ceiling := c.ws.Limits.RequestsPerMinute; rate != nil && ceiling != nil && *rate > *ceiling {
			writeProblem(w, invalid(fmt.Sprintf("«requests_per_minute» no puede pasar el límite del plan del espacio, %d por minuto.", *ceiling)))
			return
		}
		patch.OwnRequestsPerMinute = &rate
	}
	ws, err := s.Store.UpdateWorkspace(r.Context(), c.ws.ID, patch)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceOf(ws, c.role))
}

type memberView struct {
	UserID *string    `json:"user_id"`
	Email  *string    `json:"email"`
	Name   *string    `json:"name"`
	Role   model.Role `json:"role"`
}

// listMembers handles GET /workspaces/{w}/members.
func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, c *call) {
	if c.user() == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	members, err := s.Store.Members(r.Context(), c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(members, func(m store.Member) memberView {
		return memberView{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role}
	}), nil))
}

// dropKeys drops from the cache the keys a closed state revoked, and logs
// the organizations it sent back to the platform's space.
func (s *Server) dropKeys(ctx context.Context, res store.StateResult) {
	for _, h := range res.RevokedKeys {
		s.Keys.Delete(ctx, h)
	}
	if len(res.Returned) > 0 {
		slog.InfoContext(ctx, "organizations returned to the platform", "workspace_id", res.WorkspaceID,
			"organizations", strings.Join(res.Returned, ","), "revoked_keys", len(res.RevokedKeys))
	}
}

// billingPush handles PUT /api/billing/organizations/{id}: billing pushes
// a space's state (cloud). 204 applied or stale; 404 with
// UnknownOrganizationType for a space Sign doesn't have.
func (s *Server) billingPush(w http.ResponseWriter, r *http.Request) {
	if !s.Cloud || s.BillingPushToken == "" {
		writeProblem(w, errNotFound)
		return
	}
	if !billing.IsPush(r.Header.Get("Authorization"), s.BillingPushToken) {
		writeProblem(w, problem(http.StatusUnauthorized, "unauthorized", "Esta ruta es solo para billing de Sello."))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeProblem(w, invalid("No se pudo leer el cuerpo."))
		return
	}
	st, err := billing.ParseState(body)
	if err != nil || st.WorkspaceID != r.PathValue("sello_workspace_id") {
		writeProblem(w, invalid("El cuerpo no es el estado de esta organización."))
		return
	}
	res, err := s.Store.ApplyState(r.Context(), st, s.PlatformWorkspace)
	if err != nil {
		internalError(w, r, err)
		return
	}
	applied, id := res.Applied, res.WorkspaceID
	s.dropKeys(r.Context(), res)
	if applied == billing.AppliedUnknown {
		p := problem(http.StatusNotFound, UnknownOrganizationType, "Sign no tiene un espacio de esta organización.")
		writeProblem(w, p)
		return
	}
	slog.InfoContext(r.Context(), "billing push", "workspace_id", id, "version", st.Version, "applied", string(applied))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
