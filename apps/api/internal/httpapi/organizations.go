package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"slices"
	"strings"

	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

var (
	errSelloLinked   = problem(http.StatusConflict, "sello-workspace-linked", "Esa organización de Sello ya está vinculada a otra organización.")
	errSelloMismatch = problem(http.StatusConflict, "organization-mismatch", "Esa organización de Sello ya está en Sign con otro NIT o de otro país.")
	errSelloLink     = forbidden("Solo el espacio de la plataforma, o quien administra esa organización en Sello, la vincula.")
)

// mayLinkSello says whether the caller may name selloID, an organization
// of Sello's IdP, on an organization: Sello's platform space, or a person
// who manages that organization at the IdP.
func (s *Server) mayLinkSello(c *call, selloID string) bool {
	if s.PlatformWorkspace != "" && c.ws.ID == s.PlatformWorkspace {
		return true
	}
	u := c.user()
	return u != nil && slices.Contains(selloAdminOf(u), selloID)
}

func validEmail(p *string) bool {
	if p == nil {
		return true
	}
	a, err := mail.ParseAddress(*p)
	return err == nil && a.Address == *p
}

// listOrganizations handles GET /workspaces/{w}/organizations.
func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeOrganizationsRead); p != nil {
		writeProblem(w, p)
		return
	}
	pg, p := page(r)
	if p != nil {
		writeProblem(w, p)
		return
	}
	f := store.OrganizationFilter{Only: c.only}
	switch rel := r.URL.Query().Get("relation"); rel {
	case "":
	case string(model.Owned), string(model.Delegated):
		f.Relation = model.Relation(rel)
	default:
		writeProblem(w, invalid("«relation» debe ser owned o delegated."))
		return
	}
	orgs, next, err := s.Store.Organizations(r.Context(), c.ws.ID, f, pg)
	if err != nil {
		internalError(w, r, err)
		return
	}
	views, err := s.organizationsOf(r.Context(), orgs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(views, next))
}

// createOrganization handles POST /workspaces/{w}/organizations.
func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeOrganizationsWrite); p != nil {
		writeProblem(w, p)
		return
	}
	if c.only != "" {
		writeProblem(w, forbidden("Una clave de una sola organización no crea otras."))
		return
	}
	var body struct {
		Name             *string `json:"name"`
		TaxID            *string `json:"tax_id"`
		Country          *string `json:"country"`
		ContactEmail     *string `json:"contact_email"`
		SelloWorkspaceID *string `json:"sello_workspace_id"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	name, taxID := trimmed(body.Name), trimmed(body.TaxID)
	if name == nil || len(*name) > 200 {
		writeProblem(w, invalid("«name» es obligatorio, de hasta 200 caracteres."))
		return
	}
	if taxID == nil || len(*taxID) > 40 || model.TaxIDKey(*taxID) == "" {
		writeProblem(w, invalid("«tax_id» es obligatorio, de hasta 40 caracteres."))
		return
	}
	country, p := s.countryOf(body.Country)
	if p != nil {
		writeProblem(w, p)
		return
	}
	if country == nil {
		writeProblem(w, invalid("«country» es obligatorio."))
		return
	}
	if _, err := s.Signer.Country(*country); err != nil {
		writeProblem(w, errCountryNotEnabled)
		return
	}
	email := trimmed(body.ContactEmail)
	if !validEmail(email) {
		writeProblem(w, invalid("«contact_email» no es un correo válido."))
		return
	}
	sello := trimmed(body.SelloWorkspaceID)
	if sello != nil && !s.mayLinkSello(c, *sello) {
		writeProblem(w, errSelloLink)
		return
	}
	org, created, err := s.Store.CreateOrganization(r.Context(), store.NewOrganization{
		WorkspaceID: c.ws.ID, Name: *name, TaxID: *taxID, Country: *country, ContactEmail: email, SelloWorkspaceID: sello,
	})
	if errors.Is(err, store.ErrSelloLinked) && sello != nil && s.PlatformWorkspace != "" && c.ws.ID == s.PlatformWorkspace {
		// The company used Sign first: the platform takes the locked
		// delegation of its organization instead of a second one.
		org, err = s.Store.DelegateLinked(r.Context(), s.PlatformWorkspace, *sello, *taxID, *country)
		if err == nil {
			slog.InfoContext(r.Context(), "platform delegated a linked organization", "organization_id", org.ID, "owner_id", org.WorkspaceID)
		}
	}
	if errors.Is(err, store.ErrSelloLinked) {
		writeProblem(w, errSelloLinked)
		return
	}
	if errors.Is(err, store.ErrSelloMismatch) {
		writeProblem(w, errSelloMismatch)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	view, err := s.organizationOf(r.Context(), org)
	if err != nil {
		internalError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		slog.InfoContext(r.Context(), "organization created", "workspace_id", c.ws.ID, "organization_id", org.ID)
	}
	writeJSON(w, status, view)
}

// getOrganization handles GET /workspaces/{w}/organizations/{o}.
func (s *Server) getOrganization(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeOrganizationsRead); p != nil {
		writeProblem(w, p)
		return
	}
	view, err := s.organizationOf(r.Context(), c.org)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// updateOrganization handles PATCH: only its owner.
func (s *Server) updateOrganization(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeOrganizationsWrite); p != nil {
		writeProblem(w, p)
		return
	}
	if c.org.Reach.Relation != model.Owned {
		writeProblem(w, errNotOwner)
		return
	}
	var body struct {
		Name             *string        `json:"name"`
		ContactEmail     optionalString `json:"contact_email"`
		SelloWorkspaceID optionalString `json:"sello_workspace_id"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	var patch store.OrganizationPatch
	if body.Name != nil {
		name := trimmed(body.Name)
		if name == nil || len(*name) > 200 {
			writeProblem(w, invalid("«name» no puede quedar vacío y tiene hasta 200 caracteres."))
			return
		}
		patch.Name = name
	}
	if body.ContactEmail.Set {
		email := trimmed(body.ContactEmail.Value)
		if !validEmail(email) {
			writeProblem(w, invalid("«contact_email» no es un correo válido."))
			return
		}
		patch.ContactEmail = &email
	}
	if body.SelloWorkspaceID.Set {
		sello := trimmed(body.SelloWorkspaceID.Value)
		// Linking needs the right over the new one; unlinking, over the
		// one it had.
		check := sello
		if check == nil {
			check = c.org.SelloWorkspaceID
		}
		if check != nil && !s.mayLinkSello(c, *check) {
			writeProblem(w, errSelloLink)
			return
		}
		patch.SelloWorkspaceID = &sello
	}
	err := s.Store.UpdateOrganization(r.Context(), c.org.ID, patch)
	if errors.Is(err, store.ErrSelloLinked) {
		writeProblem(w, errSelloLinked)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	org, err := s.Store.ResolveOrganization(r.Context(), c.ws.ID, c.org.ID, "")
	if err != nil {
		internalError(w, r, err)
		return
	}
	view, err := s.organizationOf(r.Context(), org)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// deleteOrganization handles DELETE: only its owner, and not while it's
// delegated.
func (s *Server) deleteOrganization(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeOrganizationsWrite); p != nil {
		writeProblem(w, p)
		return
	}
	if c.org.Reach.Relation != model.Owned {
		writeProblem(w, errNotOwner)
		return
	}
	certs, err := s.Store.Certificates(r.Context(), c.org.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	err = s.Store.DeleteOrganization(r.Context(), c.org.ID)
	if errors.Is(err, store.ErrHasDelegations) {
		writeProblem(w, problem(http.StatusConflict, "organization-has-delegations", "La organización está delegada a otro espacio: quite antes sus delegaciones."))
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	for _, cert := range certs {
		s.Signer.Forget(cert.ID)
	}
	slog.InfoContext(r.Context(), "organization deleted", "workspace_id", c.ws.ID, "organization_id", c.org.ID)
	w.WriteHeader(http.StatusNoContent)
}

// claimChecks are what both claim routes ask: the cloud, a person who
// manages the space, and a space that isn't the platform's. It returns the
// organizations of the IdP the person manages.
func (s *Server) claimChecks(w http.ResponseWriter, c *call) ([]string, bool) {
	if !s.Cloud || s.PlatformWorkspace == "" {
		writeProblem(w, errNotCloud)
		return nil, false
	}
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return nil, false
	}
	if !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio trae organizaciones."))
		return nil, false
	}
	return selloAdminOf(u), true
}

// claimableOrganizations handles GET /workspaces/{w}/claimable-organizations.
func (s *Server) claimableOrganizations(w http.ResponseWriter, r *http.Request, c *call) {
	managed, ok := s.claimChecks(w, c)
	if !ok {
		return
	}
	var orgs []store.Organization
	if c.ws.ID != s.PlatformWorkspace {
		var err error
		orgs, err = s.Store.ClaimableOrganizations(r.Context(), s.PlatformWorkspace, managed)
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	views, err := s.organizationsOf(r.Context(), orgs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(views, nil))
}

// claimOrganizations handles POST /workspaces/{w}/organization-claims:
// «Usar en Sello Sign».
func (s *Server) claimOrganizations(w http.ResponseWriter, r *http.Request, c *call) {
	managed, ok := s.claimChecks(w, c)
	if !ok {
		return
	}
	var body struct {
		OrganizationIDs []string `json:"organization_ids"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	if len(body.OrganizationIDs) == 0 || len(body.OrganizationIDs) > 100 {
		writeProblem(w, invalid("«organization_ids» lleva de 1 a 100 ids."))
		return
	}
	notClaimable := problem(http.StatusNotFound, "organization-not-claimable", "Alguna de las organizaciones no está en el espacio de Sello o no la administra en Sello.")
	if c.ws.ID == s.PlatformWorkspace {
		writeProblem(w, notClaimable)
		return
	}
	orgs, err := s.Store.ClaimOrganizations(r.Context(), c.ws.ID, s.PlatformWorkspace, body.OrganizationIDs, managed)
	switch {
	case errors.Is(err, store.ErrNotClaimable):
		writeProblem(w, notClaimable)
		return
	case errors.Is(err, store.ErrOrganizationExists):
		writeProblem(w, problem(http.StatusConflict, "organization-exists", "El espacio ya tiene una organización con ese país y NIT."))
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	idList := make([]string, len(orgs))
	for i, o := range orgs {
		idList[i] = o.ID
	}
	slog.InfoContext(r.Context(), "organizations claimed", "workspace_id", c.ws.ID, "organizations", strings.Join(idList, ","))
	views, err := s.organizationsOf(r.Context(), orgs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(views, nil))
}
