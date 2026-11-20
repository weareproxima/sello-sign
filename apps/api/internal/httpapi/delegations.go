package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

var errDelegationNotFound = problem(http.StatusNotFound, "delegation-not-found", "No existe esa delegación de la organización.")

// listDelegations handles GET …/delegations: only the owner.
func (s *Server) listDelegations(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeDelegationsRead); p != nil {
		writeProblem(w, p)
		return
	}
	if c.org.Reach.Relation != model.Owned {
		writeProblem(w, errNotOwner)
		return
	}
	ds, err := s.Store.Delegations(r.Context(), c.org.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(ds, delegationOf), nil))
}

// createDelegation handles POST …/delegations: only the owner.
func (s *Server) createDelegation(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeDelegationsWrite); p != nil {
		writeProblem(w, p)
		return
	}
	if c.org.Reach.Relation != model.Owned {
		writeProblem(w, errNotOwner)
		return
	}
	var body struct {
		WorkspaceID string   `json:"workspace_id"`
		Scopes      []string `json:"scopes"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	scopes, ok := model.ParseScopes(body.Scopes, model.DelegationScopes)
	if !ok {
		writeProblem(w, invalid("«scopes» lleva uno o más de sign, certificates:read y certificates:write."))
		return
	}
	target := canonicalOr(body.WorkspaceID)
	if target == "" {
		writeProblem(w, invalid("«workspace_id» no es un id."))
		return
	}
	if target == c.ws.ID {
		writeProblem(w, invalid("Un espacio no se delega sus propias organizaciones."))
		return
	}
	if _, err := s.Store.Workspace(r.Context(), target); errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errWorkspaceNotFound)
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	d, err := s.Store.CreateDelegation(r.Context(), c.org.ID, target, scopes, false)
	if errors.Is(err, store.ErrDelegationExists) {
		writeProblem(w, problem(http.StatusConflict, "delegation-exists", "Ese espacio ya tiene una delegación de la organización."))
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "delegation created", "organization_id", c.org.ID, "delegation_id", d.ID, "to", target)
	writeJSON(w, http.StatusCreated, delegationOf(d))
}

// deleteDelegation handles DELETE …/delegations/{d}: the owner removes one
// that isn't locked; the delegate renounces its own, locked or not.
func (s *Server) deleteDelegation(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeDelegationsWrite); p != nil {
		writeProblem(w, p)
		return
	}
	id := canonicalOr(r.PathValue("delegation_id"))
	var force bool
	switch {
	case c.org.Reach.Relation == model.Owned:
	case c.org.Delegation != nil && c.org.Delegation.ID == id:
		force = true
	default:
		writeProblem(w, errDelegationNotFound)
		return
	}
	err := s.Store.DeleteDelegation(r.Context(), c.org.ID, id, force)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errDelegationNotFound)
		return
	case errors.Is(err, store.ErrDelegationLocked):
		writeProblem(w, problem(http.StatusConflict, "delegation-locked", "Esta delegación la conserva Sello mientras la organización emite con la plataforma: solo Sello la quita."))
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "delegation removed", "organization_id", c.org.ID, "delegation_id", id, "by_delegate", force)
	w.WriteHeader(http.StatusNoContent)
}
