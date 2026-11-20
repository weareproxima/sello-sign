package httpapi

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/store"
)

var errCertificateNotFound = problem(http.StatusNotFound, "certificate-not-found", "No existe ese certificado en la organización.")

// maxCertificate caps an uploaded file, decoded.
const maxCertificate = 1 << 20

// listCertificates handles GET …/certificates.
func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.requireOnOrganization(model.ScopeCertificatesRead); p != nil {
		writeProblem(w, p)
		return
	}
	certs, err := s.Store.Certificates(r.Context(), c.org.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(certs, certificateOf), nil))
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("not base64")
}

// uploadCertificate handles POST …/certificates: the country reads the
// file, its tax id must be the organization's, and its material is sealed.
func (s *Server) uploadCertificate(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.requireOnOrganization(model.ScopeCertificatesWrite); p != nil {
		writeProblem(w, p)
		return
	}
	var body struct {
		Environment string  `json:"environment"`
		File        string  `json:"file"`
		Password    *string `json:"password"`
		Activate    *bool   `json:"activate"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	env, ok := model.ParseEnvironment(body.Environment)
	if !ok {
		writeProblem(w, invalid("«environment» debe ser test o prod."))
		return
	}
	data, err := decodeBase64(body.File)
	if err != nil || len(data) == 0 {
		writeProblem(w, invalid("«file» debe ser el archivo en base64."))
		return
	}
	defer clear(data)
	if len(data) > maxCertificate {
		writeProblem(w, problem(http.StatusRequestEntityTooLarge, "payload-too-large", "El archivo es demasiado grande para ser un certificado."))
		return
	}
	country, err := s.Signer.Country(c.org.Country)
	if err != nil {
		writeProblem(w, errCountryNotEnabled)
		return
	}
	upload := signing.Upload{Data: data}
	if body.Password != nil {
		upload.Password = *body.Password
	}
	material, info, err := country.ParseCertificate(upload)
	if err != nil {
		if p := signingProblem(err); p != nil {
			writeProblem(w, p)
			return
		}
		// A country's parser may fail in ways it didn't classify: the file
		// is still the client's to fix. Never the error's text, which may
		// quote the file.
		slog.InfoContext(r.Context(), "certificate refused", "organization_id", c.org.ID, "country", c.org.Country)
		writeProblem(w, problem(http.StatusUnprocessableEntity, "invalid-certificate", "El país no pudo leer el archivo como un certificado, o la contraseña no es la suya."))
		return
	}
	if info.TaxID != "" && model.TaxIDKey(info.TaxID) != model.TaxIDKey(c.org.TaxID) {
		writeProblem(w, problem(http.StatusUnprocessableEntity, "certificate-tax-id-mismatch", "El certificado es de otro contribuyente: su NIT no es el de la organización."))
		return
	}
	id := ids.New()
	sealed, err := s.Signer.Seal(id, material)
	if err != nil {
		internalError(w, r, errors.New("could not seal a certificate's material"))
		return
	}
	cert := store.Certificate{
		ID: id, OrganizationID: c.org.ID, Environment: env, Subject: info.Subject, Issuer: info.Issuer, Serial: info.Serial,
	}
	if info.TaxID != "" {
		cert.TaxID = &info.TaxID
	}
	if !info.NotBefore.IsZero() {
		cert.NotBefore = &info.NotBefore
	}
	if !info.NotAfter.IsZero() {
		cert.NotAfter = &info.NotAfter
	}
	activate := body.Activate == nil || *body.Activate
	saved, err := s.Store.InsertCertificate(r.Context(), c.ws.ID, cert, sealed, activate)
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "certificate uploaded", "organization_id", c.org.ID, "certificate_id", id,
		"environment", string(env), "active", activate)
	writeJSON(w, http.StatusCreated, certificateOf(saved))
}

// updateCertificate handles PATCH …/certificates/{c}: activate or
// deactivate.
func (s *Server) updateCertificate(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.requireOnOrganization(model.ScopeCertificatesWrite); p != nil {
		writeProblem(w, p)
		return
	}
	var body struct {
		Active *bool `json:"active"`
	}
	if p := decode(w, r, &body); p != nil {
		writeProblem(w, p)
		return
	}
	if body.Active == nil {
		writeProblem(w, invalid("Falta «active»."))
		return
	}
	cert, err := s.Store.SetCertificateActive(r.Context(), c.ws.ID, c.org.ID, r.PathValue("certificate_id"), *body.Active)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errCertificateNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, certificateOf(cert))
}

// deleteCertificate handles DELETE …/certificates/{c}.
func (s *Server) deleteCertificate(w http.ResponseWriter, r *http.Request, c *call) {
	if p := c.requireOnOrganization(model.ScopeCertificatesWrite); p != nil {
		writeProblem(w, p)
		return
	}
	id := r.PathValue("certificate_id")
	err := s.Store.DeleteCertificate(r.Context(), c.ws.ID, c.org.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, errCertificateNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.Signer.Forget(canonicalOr(id))
	w.WriteHeader(http.StatusNoContent)
}
