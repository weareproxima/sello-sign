package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/store"
)

// maxDocument caps a document to sign.
const maxDocument = 10 << 20

type signedView struct {
	Signature    string `json:"signature"`
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
}

type verificationView struct {
	Valid        bool    `json:"valid"`
	Document     *string `json:"document"`
	DocumentID   string  `json:"document_id"`
	DocumentType string  `json:"document_type"`
}

// signingProblem maps what signing failed with to its problem; nil if it's
// an internal error.
func signingProblem(err error) *Problem {
	switch {
	case errors.Is(err, ErrCountryNotEnabled):
		return errCountryNotEnabled
	case errors.Is(err, ErrNoActiveCertificate):
		return problem(http.StatusConflict, "certificate-not-active", "La organización no tiene un certificado activo en este ambiente.")
	case errors.Is(err, ErrCertificateExpired):
		return problem(http.StatusConflict, "certificate-expired", "El certificado activo de la organización en este ambiente venció.")
	case errors.Is(err, signing.ErrNotImplemented):
		return problem(http.StatusNotImplemented, "country-not-implemented", "El firmador de este país todavía no está disponible.")
	case errors.Is(err, signing.ErrUnsupportedContentType):
		return problem(http.StatusUnsupportedMediaType, "unsupported-content-type", "Este país no firma documentos de ese tipo de contenido.")
	case errors.Is(err, signing.ErrInvalidDocument):
		return problem(http.StatusUnprocessableEntity, "invalid-document", "El cuerpo no es un documento que este país pueda firmar.")
	case errors.Is(err, signing.ErrInvalidCertificate):
		return problem(http.StatusUnprocessableEntity, "invalid-certificate", "El país no pudo leer el archivo como un certificado.")
	}
	return nil
}

// documentType reads the media type of a document to sign:
// application/json or application/xml (text/xml is the latter).
func documentType(r *http.Request) (string, *Problem) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case err != nil:
		return "", problem(http.StatusUnsupportedMediaType, "unsupported-content-type", "Falta Content-Type: application/json o application/xml.")
	case mt == "application/json":
		return mt, nil
	case mt == "application/xml" || mt == "text/xml":
		return "application/xml", nil
	}
	return "", problem(http.StatusUnsupportedMediaType, "unsupported-content-type", "El documento debe ser application/json o application/xml.")
}

func readDocument(w http.ResponseWriter, r *http.Request) ([]byte, *Problem) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocument))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, problem(http.StatusRequestEntityTooLarge, "payload-too-large", "El documento es demasiado grande.")
	}
	if err != nil {
		return nil, invalid("No se pudo leer el cuerpo.")
	}
	if len(body) == 0 {
		return nil, invalid("El cuerpo está vacío: debe ser el documento.")
	}
	return body, nil
}

// keyOrganization resolves X-Organization-Id among what a key reaches,
// and checks the reach lets it sign.
func (s *Server) keyOrganization(w http.ResponseWriter, r *http.Request, c *call) (store.Organization, bool) {
	k := c.key()
	if k == nil {
		writeProblem(w, errKeyOnly)
		return store.Organization{}, false
	}
	if !model.HasScope(k.Scopes, model.ScopeSign) {
		writeProblem(w, forbidden("La clave no tiene el permiso «sign»."))
		return store.Organization{}, false
	}
	ref := r.Header.Get("X-Organization-Id")
	if ref == "" {
		writeProblem(w, problem(http.StatusBadRequest, "organization-required", "Falta X-Organization-Id: el id de la organización o su NIT."))
		return store.Organization{}, false
	}
	org, err := s.Store.ResolveOrganization(r.Context(), k.WorkspaceID, ref, k.OrganizationID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errOrgNotFound)
		return store.Organization{}, false
	case errors.Is(err, store.ErrAmbiguous):
		writeProblem(w, problem(http.StatusConflict, "organization-ambiguous", "Ese NIT nombra más de una organización: use su id."))
		return store.Organization{}, false
	case err != nil:
		internalError(w, r, err)
		return store.Organization{}, false
	}
	if !org.Reach.Allows(model.ScopeSign) {
		writeProblem(w, forbidden("La delegación de esta organización no permite firmar."))
		return store.Organization{}, false
	}
	return org, true
}

// signAndRecord signs body for org in env, logs it and counts it in the
// signing space, if its limits admit it. k is the key that signs; nil from
// the panel, with userID.
func (s *Server) signAndRecord(w http.ResponseWriter, r *http.Request, org store.Organization, signer string, env model.Environment, k *auth.Key, userID string) {
	ct, prob := documentType(r)
	if prob != nil {
		writeProblem(w, prob)
		return
	}
	body, prob := readDocument(w, r)
	if prob != nil {
		writeProblem(w, prob)
		return
	}
	if !s.admit(w, r, signer, env, k) {
		return
	}
	keyID := ""
	if k != nil {
		keyID = k.ID
	}
	start := time.Now()
	signed, err := s.Signer.Sign(r.Context(), org, env, body, ct)
	elapsed := time.Since(start)
	if err != nil {
		if p := signingProblem(err); p != nil {
			writeProblem(w, p)
			return
		}
		internalError(w, r, err)
		return
	}
	if _, err := s.Store.RecordSignature(r.Context(), store.NewSignature{
		OrganizationID: org.ID, SignedByWorkspaceID: signer, APIKeyID: keyID, UserID: userID, Environment: env,
		DocumentID: signed.Document.ID, DocumentType: signed.Document.Type, Duration: elapsed, At: s.Now(),
	}); err != nil {
		// The signature is good; only its log failed. It's answered, and
		// the failure is loud.
		slog.ErrorContext(r.Context(), "could not record a signature", "organization_id", org.ID, "workspace_id", signer, "error", err)
	}
	writeJSON(w, http.StatusOK, signedView{
		Signature: string(signed.Signature), DocumentID: signed.Document.ID, DocumentType: signed.Document.Type,
	})
}

// signWithKey handles POST /signatures.
func (s *Server) signWithKey(w http.ResponseWriter, r *http.Request, c *call) {
	org, ok := s.keyOrganization(w, r, c)
	if !ok {
		return
	}
	k := c.key()
	s.signAndRecord(w, r, org, k.WorkspaceID, k.Environment, k, "")
}

// verify handles POST /verifications: with a key that signs, against its
// environment; with the panel's token, any member of a space that owns the
// organization or has it delegated (with sign or certificates:read),
// against the environment the body names. Nothing is kept: neither the
// document nor the result.
func (s *Server) verify(w http.ResponseWriter, r *http.Request, c *call) {
	var body struct {
		Signature   string  `json:"signature"`
		Environment *string `json:"environment"`
	}
	var (
		org store.Organization
		env model.Environment
	)
	if k := c.key(); k != nil {
		var ok bool
		if org, ok = s.keyOrganization(w, r, c); !ok {
			return
		}
		if p := decode(w, r, &body); p != nil {
			writeProblem(w, p)
			return
		}
		env = k.Environment
		if body.Environment != nil && *body.Environment != string(env) {
			writeProblem(w, invalid("«environment» es el de la clave; con esta, «"+string(env)+"»."))
			return
		}
	} else {
		var ok bool
		if org, ok = s.personOrganization(w, r, c); !ok {
			return
		}
		if p := decode(w, r, &body); p != nil {
			writeProblem(w, p)
			return
		}
		if body.Environment == nil {
			writeProblem(w, invalid("Falta «environment»: test o prod."))
			return
		}
		if env, ok = model.ParseEnvironment(*body.Environment); !ok {
			writeProblem(w, invalid("«environment» debe ser test o prod."))
			return
		}
	}
	if body.Signature == "" {
		writeProblem(w, invalid("Falta «signature»."))
		return
	}
	v, err := s.Signer.Verify(r.Context(), org, env, []byte(body.Signature))
	if err != nil {
		if p := signingProblem(err); p != nil {
			writeProblem(w, p)
			return
		}
		internalError(w, r, err)
		return
	}
	out := verificationView{Valid: v.Valid, DocumentID: v.Document.ID, DocumentType: v.Document.Type}
	if v.Payload != nil {
		doc := string(v.Payload)
		out.Document = &doc
	}
	writeJSON(w, http.StatusOK, out)
}

// personOrganization resolves X-Organization-Id among the organizations a
// person reaches through their spaces, and checks they may verify with it:
// the owner's members, and a delegate's if its delegation signs or reads
// the certificates.
func (s *Server) personOrganization(w http.ResponseWriter, r *http.Request, c *call) (store.Organization, bool) {
	u := c.user()
	ref := strings.TrimSpace(r.Header.Get("X-Organization-Id"))
	if ref == "" {
		writeProblem(w, problem(http.StatusBadRequest, "organization-required", "Falta X-Organization-Id: el id de la organización o su NIT."))
		return store.Organization{}, false
	}
	org, _, err := s.Store.ResolveOrganizationForPerson(r.Context(), u.Subject, u.SelloSub, ref)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, errOrgNotFound)
		return store.Organization{}, false
	case errors.Is(err, store.ErrAmbiguous):
		writeProblem(w, problem(http.StatusConflict, "organization-ambiguous", "Ese NIT nombra más de una organización: use su id."))
		return store.Organization{}, false
	case err != nil:
		internalError(w, r, err)
		return store.Organization{}, false
	}
	if !org.Reach.Allows(model.ScopeSign) && !org.Reach.Allows(model.ScopeCertificatesRead) {
		writeProblem(w, forbidden("La delegación de esta organización no permite verificar con su certificado."))
		return store.Organization{}, false
	}
	return org, true
}

// signFromPanel handles POST /workspaces/{w}/organizations/{o}/signatures:
// a person signs to try it, members only in test.
func (s *Server) signFromPanel(w http.ResponseWriter, r *http.Request, c *call) {
	u := c.user()
	if u == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	env, ok := model.ParseEnvironment(r.URL.Query().Get("environment"))
	if !ok {
		writeProblem(w, invalid("«environment» debe ser test o prod."))
		return
	}
	if !model.CanSignFromPanel(c.role, env) {
		writeProblem(w, forbidden("Su rol solo firma en el ambiente de prueba."))
		return
	}
	if !c.org.Reach.Allows(model.ScopeSign) {
		writeProblem(w, forbidden("La delegación de esta organización no permite firmar."))
		return
	}
	s.signAndRecord(w, r, c.org, c.ws.ID, env, nil, u.Subject)
}

// listSignatures handles GET /workspaces/{w}/signatures.
func (s *Server) listSignatures(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeLogsRead); p != nil {
		writeProblem(w, p)
		return
	}
	pg, p := page(r)
	if p != nil {
		writeProblem(w, p)
		return
	}
	q := r.URL.Query()
	f := store.SignatureFilter{Only: c.only}
	if v := q.Get("organization_id"); v != "" {
		id := canonicalOr(v)
		if id == "" {
			writeProblem(w, invalid("«organization_id» no es un id."))
			return
		}
		f.OrganizationID = id
	}
	if v := q.Get("environment"); v != "" {
		env, ok := model.ParseEnvironment(v)
		if !ok {
			writeProblem(w, invalid("«environment» debe ser test o prod."))
			return
		}
		f.Environment = env
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeProblem(w, invalid("«"+name+"» debe ser una fecha RFC 3339 (2026-11-19T00:00:00Z)."))
				return
			}
			*dst = &t
		}
	}
	items, next, err := s.Store.Signatures(r.Context(), c.ws.ID, f, pg)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(items, signatureOf), next))
}

// getSignature handles GET /workspaces/{w}/signatures/{s}.
func (s *Server) getSignature(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeLogsRead); p != nil {
		writeProblem(w, p)
		return
	}
	e, err := s.Store.Signature(r.Context(), c.ws.ID, r.PathValue("signature_id"), c.only)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, problem(http.StatusNotFound, "signature-not-found", "No existe esa firma en el registro del espacio."))
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, signatureOf(e))
}

type usageLineView struct {
	Month       string            `json:"month"`
	Environment model.Environment `json:"environment"`
	Signatures  int64             `json:"signatures"`
}

type usageView struct {
	Plan   *string         `json:"plan"`
	Limits store.Limits    `json:"limits"`
	Items  []usageLineView `json:"items"`
}

// usage handles GET /workspaces/{w}/usage.
func (s *Server) usage(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.require(model.ScopeLogsRead); p != nil {
		writeProblem(w, p)
		return
	}
	now := s.Now().UTC()
	to := model.Month(now)
	from := model.Month(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0))
	q := r.URL.Query()
	for name, dst := range map[string]*string{"from": &from, "to": &to} {
		if v := q.Get(name); v != "" {
			if _, err := time.Parse("2006-01", v); err != nil {
				writeProblem(w, invalid("«"+name+"» debe ser un mes AAAA-MM."))
				return
			}
			*dst = v
		}
	}
	lines, err := s.Store.Usage(r.Context(), c.ws.ID, from, to)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, usageView{Plan: c.ws.Plan, Limits: c.ws.Limits, Items: mapSlice(lines, func(l store.UsageLine) usageLineView {
		return usageLineView{Month: l.Month, Environment: l.Environment, Signatures: l.Signatures}
	})})
}
