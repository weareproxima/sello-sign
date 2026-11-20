package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// call is a request with its caller and, under /workspaces/{w}, its space.
type call struct {
	p *auth.Principal
	// ws is the space of the path; role, the person's in it ("" for a key).
	ws   store.Workspace
	role model.Role
	// scopes are what the caller may do in the space: the key's, or the
	// person's role's.
	scopes []model.Scope
	// only is the single organization a key of one organization reaches.
	only string
	// org is the organization of the path, under …/organizations/{o}.
	org store.Organization
}

func (c *call) key() *auth.Key   { return c.p.Key }
func (c *call) user() *auth.User { return c.p.User }

// require answers 403 if the caller lacks scope.
func (c *call) require(scope model.Scope) *Problem {
	if model.HasScope(c.scopes, scope) {
		return nil
	}
	if c.user() != nil {
		return forbidden("Su rol en el espacio no permite hacer esto.")
	}
	return forbidden("La clave no tiene el permiso «" + string(scope) + "».")
}

// requireOnOrganization is require plus, for a delegated organization,
// what the delegation lets the space do.
func (c *call) requireOnOrganization(scope model.Scope) *Problem {
	if p := c.require(scope); p != nil {
		return p
	}
	if !c.org.Reach.Allows(scope) {
		return forbidden("La delegación de esta organización no permite hacer esto.")
	}
	return nil
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, c *call)

// authenticate reads the caller's credentials: X-API-Key, else a Bearer
// token of the panel.
func (s *Server) authenticate(r *http.Request) (*auth.Principal, *Problem) {
	ctx := r.Context()
	if key := r.Header.Get("X-API-Key"); key != "" {
		if _, ok := auth.KeyEnvironment(key); !ok {
			return nil, errUnauthorized
		}
		hash := auth.HashKey(key)
		k, ok := s.Keys.Get(ctx, hash)
		if !ok {
			var err error
			k, err = s.Store.KeyByHash(ctx, hash)
			if errors.Is(err, store.ErrNotFound) {
				return nil, errUnauthorized
			}
			if err != nil {
				slog.ErrorContext(ctx, "key lookup", "error", err)
				return nil, errInternal
			}
			s.Keys.Set(ctx, hash, k)
		}
		s.touch(k.ID)
		return &auth.Principal{Key: &k}, nil
	}
	header := r.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || s.Verifier == nil {
		return nil, errUnauthorized
	}
	claims, err := s.Verifier.Verify(ctx, strings.TrimSpace(token))
	switch {
	case errors.Is(err, auth.ErrExpiredToken):
		return nil, errTokenExpired
	case errors.Is(err, auth.ErrKeysUnavailable):
		slog.WarnContext(ctx, "panel keys unavailable", "reason", auth.RefusalReason(err))
		return nil, errKeysUnavailable
	case err != nil:
		slog.InfoContext(ctx, "token refused", "reason", auth.RefusalReason(err))
		return nil, errUnauthorized
	}
	return &auth.Principal{User: &auth.User{Claims: claims}}, nil
}

// touchEvery bounds how often a key's last_used_at is written.
const touchEvery = time.Minute

// touch records a key's use, at most once a minute per key and without
// making the request wait.
func (s *Server) touch(id string) {
	now := s.Now()
	s.touchMu.Lock()
	if last, ok := s.touched[id]; ok && now.Sub(last) < touchEvery {
		s.touchMu.Unlock()
		return
	}
	if len(s.touched) > 10_000 {
		clear(s.touched)
	}
	s.touched[id] = now
	s.touchMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Store.TouchAPIKey(ctx, id); err != nil {
			slog.Warn("could not record a key's use", "error", err)
		}
	}()
}

// authed runs h with the caller.
func (s *Server) authed(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, prob := s.authenticate(r)
		if prob != nil {
			writeProblem(w, prob)
			return
		}
		h(w, r, &call{p: p})
	}
}

// inWorkspace runs h within the space of the path: a key's own (or
// "self"), or one the person is in. Anything else is 404, the same as a
// space that doesn't exist.
func (s *Server) inWorkspace(h handlerFunc) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, c *call) {
		ref := r.PathValue("workspace_id")
		ctx := r.Context()
		var id string
		if k := c.key(); k != nil {
			if ref != "self" && !strings.EqualFold(ref, k.WorkspaceID) {
				writeProblem(w, errWorkspaceNotFound)
				return
			}
			id, c.scopes, c.only = k.WorkspaceID, k.Scopes, k.OrganizationID
		} else {
			id = pathID(r, "workspace_id")
			if id == "" {
				writeProblem(w, errWorkspaceNotFound)
				return
			}
			u := c.user()
			role, err := s.Store.RoleIn(ctx, id, u.Subject, u.SelloSub)
			if err != nil {
				internalError(w, r, err)
				return
			}
			if role == "" {
				writeProblem(w, errWorkspaceNotFound)
				return
			}
			c.role, c.scopes = role, model.ScopesOfRole(role)
		}
		ws, err := s.Store.Workspace(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, errWorkspaceNotFound)
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		c.ws = ws
		h(w, r, c)
	})
}

// inOrganization runs h with the organization of the path, among those the
// space reaches (and, for a key of one organization, only that one).
func (s *Server) inOrganization(h handlerFunc) http.HandlerFunc {
	return s.inWorkspace(func(w http.ResponseWriter, r *http.Request, c *call) {
		id := pathID(r, "organization_id")
		if id == "" {
			writeProblem(w, errOrgNotFound)
			return
		}
		org, err := s.Store.ResolveOrganization(r.Context(), c.ws.ID, id, c.only)
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, errOrgNotFound)
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		c.org = org
		h(w, r, c)
	})
}

// selloAdminOf is the organizations of Sello's IdP the person manages
// (owner or admin), as their token says.
func selloAdminOf(u *auth.User) []string {
	var out []string
	for _, w := range u.SelloWorkspaces {
		if r, ok := model.ParseRole(w.Role); ok && r.Manages() {
			out = append(out, w.ID)
		}
	}
	return out
}
