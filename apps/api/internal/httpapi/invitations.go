package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

var (
	errInvitationNotFound = problem(http.StatusNotFound, "invitation-not-found", "No existe la invitación, o se revocó.")
	errMembersManaged     = problem(http.StatusConflict, "members-managed-by-sello",
		"Los miembros de este espacio son los de su organización en Sello: se invitan y cambian de rol en «Su cuenta › Organización › Miembros y acceso».")
	errMemberNotFound = problem(http.StatusNotFound, "member-not-found", "No hay un miembro con ese usuario en el espacio.")
	errWorkspaceFull  = problem(http.StatusConflict, "workspace-full", "El plan del espacio no admite más miembros.")
)

type invitedByView struct {
	UserID string  `json:"user_id"`
	Email  *string `json:"email"`
	Name   *string `json:"name"`
}

type invitationView struct {
	ID        string        `json:"id"`
	Email     string        `json:"email"`
	Role      model.Role    `json:"role"`
	Status    string        `json:"status"`
	InvitedBy invitedByView `json:"invited_by"`
	ExpiresAt time.Time     `json:"expires_at"`
	CreatedAt time.Time     `json:"created_at"`
	// Token is the invitation's token, only when it's created.
	Token string `json:"token,omitempty"`
}

// invitationStatus is pending, expired or accepted.
func invitationStatus(i store.Invitation, now time.Time) string {
	switch {
	case i.AcceptedAt != nil:
		return "accepted"
	case i.Expired(now):
		return "expired"
	}
	return "pending"
}

func (s *Server) invitationOf(i store.Invitation) invitationView {
	by := ""
	if i.InvitedBy.UserID != nil {
		by = *i.InvitedBy.UserID
	}
	return invitationView{ID: i.ID, Email: i.Email, Role: i.Role, Status: invitationStatus(i, s.Now()),
		InvitedBy: invitedByView{UserID: by, Email: i.InvitedBy.Email, Name: i.InvitedBy.Name},
		ExpiresAt: utc(i.ExpiresAt), CreatedAt: utc(i.CreatedAt)}
}

// managesMembers answers the problem, if any, of a person who wants to
// manage the members of the call's space: only with the token, owners and
// admins, and never in a space whose members billing pushes.
func (s *Server) managesMembers(c *call) *Problem {
	if c.user() == nil {
		return errTokenOnly
	}
	if !c.role.Manages() {
		return forbidden("Solo quien administra el espacio puede invitar y cambiar roles.")
	}
	if c.ws.SelloWorkspaceID != nil {
		return errMembersManaged
	}
	return nil
}

// listInvitations handles GET /workspaces/{w}/invitations.
func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request, c *call) {
	if c.user() == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio ve sus invitaciones."))
		return
	}
	items, err := s.Store.Invitations(r.Context(), c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(items, s.invitationOf), nil))
}

// createInvitation handles POST /workspaces/{w}/invitations: the token is
// answered once, for the panel to put it in the e-mail's link.
func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request, c *call) {
	if p := s.managesMembers(c); p != nil {
		writeProblem(w, p)
		return
	}
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	email := model.NormalizeEmail(body.Email)
	if !model.ValidEmail(email) {
		writeProblem(w, invalid("«email» debe ser un correo electrónico."))
		return
	}
	role, ok := model.ParseRole(body.Role)
	if !ok || string(role) != body.Role {
		writeProblem(w, invalid("«role» debe ser owner, admin o member."))
		return
	}
	if !model.CanInvite(c.role, role) {
		writeProblem(w, forbidden("Solo un propietario puede invitar a otro propietario."))
		return
	}
	u := c.user()
	ctx := r.Context()
	if err := s.Store.UpsertUser(ctx, u.Subject, u.Email, u.Name, u.SelloSub); err != nil {
		internalError(w, r, err)
		return
	}
	token, hash := auth.NewInvitationToken()
	inv, err := s.Store.CreateInvitation(ctx, store.NewInvitation{
		WorkspaceID: c.ws.ID, Email: email, Role: role, Hash: hash, InvitedBy: u.Subject,
	})
	switch {
	case errors.Is(err, store.ErrMembersManaged):
		writeProblem(w, errMembersManaged)
		return
	case errors.Is(err, store.ErrAlreadyMember):
		writeProblem(w, problem(http.StatusConflict, "already-member", "Esa persona ya es miembro del espacio."))
		return
	case errors.Is(err, store.ErrInvitationExists):
		writeProblem(w, problem(http.StatusConflict, "invitation-exists", "Ese correo ya tiene una invitación pendiente. Revóquela antes de enviar otra."))
		return
	case errors.Is(err, store.ErrWorkspaceFull):
		writeProblem(w, errWorkspaceFull)
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	slog.InfoContext(ctx, "invitation created", "workspace_id", c.ws.ID, "invitation_id", inv.ID, "role", string(role))
	view := s.invitationOf(inv)
	view.Token = token
	writeJSON(w, http.StatusCreated, view)
}

// revokeInvitation handles DELETE /workspaces/{w}/invitations/{i}.
func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request, c *call) {
	if p := s.managesMembers(c); p != nil {
		writeProblem(w, p)
		return
	}
	err := s.Store.RevokeInvitation(r.Context(), c.ws.ID, pathID(r, "invitation_id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errInvitationNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitation revoked", "workspace_id", c.ws.ID, "invitation_id", pathID(r, "invitation_id"))
	w.WriteHeader(http.StatusNoContent)
}

// updateMember handles PATCH /workspaces/{w}/members/{user_id}: the role of
// someone added in Sign.
func (s *Server) updateMember(w http.ResponseWriter, r *http.Request, c *call) {
	if p := s.managesMembers(c); p != nil {
		writeProblem(w, p)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	role, ok := model.ParseRole(body.Role)
	if !ok || string(role) != body.Role {
		writeProblem(w, invalid("«role» debe ser owner, admin o member."))
		return
	}
	userID := r.PathValue("user_id")
	m, err := s.Store.ChangeRole(r.Context(), c.ws.ID, userID, role, c.role)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errMemberNotFound)
		return
	case errors.Is(err, store.ErrMembersManaged):
		writeProblem(w, errMembersManaged)
		return
	case errors.Is(err, store.ErrRoleForbidden):
		writeProblem(w, forbidden("Solo un propietario puede dar o quitar el rol de propietario."))
		return
	case errors.Is(err, store.ErrLastOwner):
		writeProblem(w, problem(http.StatusConflict, "last-owner", "El espacio se quedaría sin propietario. Haga propietario a otra persona antes."))
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "member role changed", "workspace_id", c.ws.ID, "role", string(role))
	writeJSON(w, http.StatusOK, memberView{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role})
}

// renewInvitation handles POST /workspaces/{w}/invitations/{i}/renewals:
// the same invitation, expired or not, with a new token and 7 more days.
// The old link stops working; the token is answered once, as on creating.
func (s *Server) renewInvitation(w http.ResponseWriter, r *http.Request, c *call) {
	if p := s.managesMembers(c); p != nil {
		writeProblem(w, p)
		return
	}
	token, hash := auth.NewInvitationToken()
	inv, err := s.Store.RenewInvitation(r.Context(), c.ws.ID, pathID(r, "invitation_id"), hash, c.role)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errInvitationNotFound)
		return
	case errors.Is(err, store.ErrMembersManaged):
		writeProblem(w, errMembersManaged)
		return
	case errors.Is(err, store.ErrRoleForbidden):
		writeProblem(w, forbidden("Solo un propietario puede invitar a otro propietario."))
		return
	case errors.Is(err, store.ErrAlreadyMember):
		writeProblem(w, problem(http.StatusConflict, "already-member", "Esa persona ya es miembro del espacio."))
		return
	case errors.Is(err, store.ErrWorkspaceFull):
		writeProblem(w, errWorkspaceFull)
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitation renewed", "workspace_id", c.ws.ID, "invitation_id", inv.ID)
	view := s.invitationOf(inv)
	view.Token = token
	writeJSON(w, http.StatusOK, view)
}

// removeMember handles DELETE /workspaces/{w}/members/{user_id}: owners and
// admins take someone added in Sign out of the space (only an owner takes
// an owner), and anyone leaves by themselves. The space never loses its
// last owner.
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	userID := r.PathValue("user_id")
	self := userID == u.Subject
	if !self && !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio puede quitar a un miembro."))
		return
	}
	if c.ws.SelloWorkspaceID != nil {
		writeProblem(w, errMembersManaged)
		return
	}
	err := s.Store.RemoveMember(r.Context(), c.ws.ID, userID, c.role, self)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errMemberNotFound)
		return
	case errors.Is(err, store.ErrMembersManaged):
		writeProblem(w, errMembersManaged)
		return
	case errors.Is(err, store.ErrRoleForbidden):
		writeProblem(w, forbidden("Solo un propietario puede quitar a otro propietario."))
		return
	case errors.Is(err, store.ErrLastOwner):
		writeProblem(w, problem(http.StatusConflict, "last-owner", "El espacio se quedaría sin propietario. Haga propietario a otra persona antes."))
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "member removed", "workspace_id", c.ws.ID, "self", self)
	w.WriteHeader(http.StatusNoContent)
}

// invitationToken reads { "token" } of the person's request; the problem
// for anything that isn't an invitation's token is the one for a revoked
// or unknown invitation.
func invitationToken(w http.ResponseWriter, r *http.Request, c *call) ([]byte, *Problem) {
	if c.user() == nil {
		return nil, errTokenOnly
	}
	var body struct {
		Token string `json:"token"`
	}
	if p := decode(w, r, &body); p != nil {
		return nil, p
	}
	if !auth.IsInvitationToken(body.Token) {
		return nil, errInvitationNotFound
	}
	return auth.HashInvitationToken(body.Token), nil
}

type invitationLookupView struct {
	WorkspaceID   string        `json:"workspace_id"`
	WorkspaceName string        `json:"workspace_name"`
	Email         string        `json:"email"`
	Role          model.Role    `json:"role"`
	Status        string        `json:"status"`
	InvitedBy     invitedByView `json:"invited_by"`
	ExpiresAt     time.Time     `json:"expires_at"`
}

// lookupInvitation handles POST /invitation-lookups: what an invitation
// says, for the person who holds its link, before accepting it.
func (s *Server) lookupInvitation(w http.ResponseWriter, r *http.Request, c *call) {
	hash, p := invitationToken(w, r, c)
	if p != nil {
		writeProblem(w, p)
		return
	}
	inv, err := s.Store.InvitationByHash(r.Context(), hash)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errInvitationNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	v := s.invitationOf(inv)
	writeJSON(w, http.StatusOK, invitationLookupView{WorkspaceID: inv.WorkspaceID, WorkspaceName: inv.WorkspaceName,
		Email: inv.Email, Role: inv.Role, Status: v.Status, InvitedBy: v.InvitedBy, ExpiresAt: v.ExpiresAt})
}

// acceptInvitation handles POST /invitation-acceptances: the person joins
// the space with the invitation's role, if the invitation is for their
// address.
func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request, c *call) {
	hash, p := invitationToken(w, r, c)
	if p != nil {
		writeProblem(w, p)
		return
	}
	u := c.user()
	ctx := r.Context()
	if err := s.Store.UpsertUser(ctx, u.Subject, u.Email, u.Name, u.SelloSub); err != nil {
		internalError(w, r, err)
		return
	}
	inv, err := s.Store.AcceptInvitation(ctx, hash, u.Subject, u.Email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errInvitationNotFound)
		return
	case errors.Is(err, store.ErrInvitationExpired):
		writeProblem(w, problem(http.StatusGone, "invitation-expired", "La invitación venció. Pida otra a quien administra el espacio."))
		return
	case errors.Is(err, store.ErrInvitationUsed):
		writeProblem(w, problem(http.StatusGone, "invitation-used", "Otra cuenta ya aceptó esta invitación."))
		return
	case errors.Is(err, store.ErrEmailMismatch):
		writeProblem(w, problem(http.StatusForbidden, "invitation-email-mismatch", "La invitación es para otro correo. Inicie sesión con la cuenta de ese correo."))
		return
	case errors.Is(err, store.ErrMembersManaged):
		writeProblem(w, errMembersManaged)
		return
	case errors.Is(err, store.ErrWorkspaceFull):
		writeProblem(w, errWorkspaceFull)
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	role, err := s.Store.RoleIn(ctx, inv.WorkspaceID, u.Subject, u.SelloSub)
	if err != nil {
		internalError(w, r, err)
		return
	}
	ws, err := s.Store.Workspace(ctx, inv.WorkspaceID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(ctx, "invitation accepted", "workspace_id", ws.ID, "invitation_id", inv.ID)
	writeJSON(w, http.StatusOK, workspaceOf(ws, role))
}
