package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

var errKeyNotFound = problem(http.StatusNotFound, "api-key-not-found", "No existe esa clave en el espacio.")

// listAPIKeys handles GET /workspaces/{w}/api-keys.
func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeKeysRead); p != nil {
		writeProblem(w, p)
		return
	}
	pg, p := page(r)
	if p != nil {
		writeProblem(w, p)
		return
	}
	keys, next, err := s.Store.APIKeys(r.Context(), c.ws.ID, c.only, pg)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(keys, apiKeyOf), next))
}

// createAPIKey handles POST /workspaces/{w}/api-keys. A key that creates
// another gives it no more than it has.
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeKeysWrite); p != nil {
		writeProblem(w, p)
		return
	}
	var body struct {
		Name           *string  `json:"name"`
		Environment    string   `json:"environment"`
		Scopes         []string `json:"scopes"`
		OrganizationID *string  `json:"organization_id"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	name := trimmed(body.Name)
	if name == nil || len(*name) > 120 {
		writeProblem(w, invalid("«name» es obligatorio, de hasta 120 caracteres."))
		return
	}
	env, ok := model.ParseEnvironment(body.Environment)
	if !ok {
		writeProblem(w, invalid("«environment» debe ser test o prod."))
		return
	}
	scopes, ok := model.ParseScopes(body.Scopes, model.AllScopes)
	if !ok {
		writeProblem(w, invalid("«scopes» lleva uno o más permisos válidos."))
		return
	}
	var orgID *string
	if body.OrganizationID != nil && *body.OrganizationID != "" {
		org, err := s.Store.ResolveOrganization(r.Context(), c.ws.ID, canonicalOr(*body.OrganizationID), c.only)
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, errOrgNotFound)
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		orgID = &org.ID
	}
	createdBy := ""
	if k := c.key(); k != nil {
		switch {
		case !model.Subset(scopes, k.Scopes):
			writeProblem(w, forbidden("Una clave no crea otra con permisos que no tiene."))
			return
		case env != k.Environment:
			writeProblem(w, forbidden("Una clave solo crea claves de su propio ambiente."))
			return
		case k.OrganizationID != "" && (orgID == nil || *orgID != k.OrganizationID):
			writeProblem(w, forbidden("Una clave de una sola organización solo crea claves de esa organización."))
			return
		}
		createdBy = "key:" + k.ID
	} else {
		createdBy = "user:" + c.user().Subject
	}
	key, hash, prefix := auth.NewKey(env)
	saved, err := s.Store.CreateAPIKey(r.Context(), store.NewAPIKey{
		WorkspaceID: c.ws.ID, OrganizationID: orgID, Environment: env, Name: *name, Scopes: scopes,
		Hash: hash, Prefix: prefix, CreatedBy: createdBy,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "api key created", "workspace_id", c.ws.ID, "api_key_id", saved.ID, "environment", string(env))
	view := apiKeyOf(saved)
	view.Key = key
	writeJSON(w, http.StatusCreated, view)
}

// revokeAPIKey handles DELETE /workspaces/{w}/api-keys/{k}.
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeKeysWrite); p != nil {
		writeProblem(w, p)
		return
	}
	id := r.PathValue("api_key_id")
	if c.only != "" {
		k, err := s.Store.APIKey(r.Context(), c.ws.ID, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && (k.OrganizationID == nil || *k.OrganizationID != c.only)) {
			writeProblem(w, errKeyNotFound)
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	hash, err := s.Store.RevokeAPIKey(r.Context(), c.ws.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errKeyNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.Keys.Delete(r.Context(), hash)
	slog.InfoContext(r.Context(), "api key revoked", "workspace_id", c.ws.ID, "api_key_id", canonicalOr(id))
	w.WriteHeader(http.StatusNoContent)
}
