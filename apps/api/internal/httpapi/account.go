package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

var errNotSelfHosted = problem(http.StatusNotFound, "not-self-hosted",
	"En la nube de Sello Sign, la cuenta se borra escribiendo a Sello: esta ruta es de una instalación propia.")

type accountSpaceView struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Role          model.Role           `json:"role"`
	Members       int                  `json:"members"`
	Organizations int                  `json:"organizations"`
	Outcome       store.AccountOutcome `json:"outcome"`
}

type accountDeletionView struct {
	// Allowed says whether DELETE /me would delete the account now.
	Allowed    bool               `json:"allowed"`
	Workspaces []accountSpaceView `json:"workspaces"`
}

func accountDeletionOf(spaces []store.AccountSpace) accountDeletionView {
	v := accountDeletionView{Allowed: true, Workspaces: make([]accountSpaceView, len(spaces))}
	for i, a := range spaces {
		if a.Outcome.Blocks() {
			v.Allowed = false
		}
		v.Workspaces[i] = accountSpaceView{ID: a.WorkspaceID, Name: a.Name, Role: a.Role, Members: a.Members,
			Organizations: a.Organizations, Outcome: a.Outcome}
	}
	return v
}

// accountDeletion handles GET /me/account-deletion (self-hosted): what
// deleting the person's account would do to each of their spaces, before
// they confirm it.
func (s *Server) accountDeletion(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if s.Cloud {
		writeProblem(w, errNotSelfHosted)
		return
	}
	spaces, err := s.Store.AccountSpaces(r.Context(), u.Subject)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, accountDeletionOf(spaces))
}

// deleteAccount handles DELETE /me (self-hosted): what the API keeps of the
// person goes, before the panel deletes their account. They leave their
// spaces, and those they were the only member of are deleted. The only owner
// of a space with other members, or the only member of one whose
// organizations other spaces use, is 409 account-has-workspaces, and
// nothing changes.
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if s.Cloud {
		writeProblem(w, errNotSelfHosted)
		return
	}
	ctx := r.Context()
	gone, err := s.Store.DeleteAccount(ctx, u.Subject)
	if errors.Is(err, store.ErrAccountBlocked) {
		writeProblem(w, problem(http.StatusConflict, "account-has-workspaces",
			"Es el único propietario de un espacio con otros miembros, o el único miembro de uno cuyas organizaciones usan otros espacios. GET /me/account-deletion dice cuál."))
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.forget(ctx, gone.DeletedSpaces)
	deleted := 0
	for _, a := range gone.Spaces {
		if a.Outcome == store.OutcomeDelete {
			deleted++
		}
	}
	slog.InfoContext(ctx, "account deleted", "left", len(gone.Spaces)-deleted, "workspaces_deleted", deleted)
	w.WriteHeader(http.StatusNoContent)
}

// forget drops from the cache the keys of deleted spaces and organizations,
// so they stop working at once on every instance that shares Valkey, and
// forgets the material of their certificates in this process.
func (s *Server) forget(ctx context.Context, gone store.DeletedSpaces) {
	for _, h := range gone.KeyHashes {
		s.Keys.Delete(ctx, h)
	}
	for _, id := range gone.Certificates {
		s.Signer.Forget(id)
	}
}
