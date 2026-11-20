// Package httpapi is the API's HTTP: the routes of contracts/README.md on
// the standard library's ServeMux, who may call each one and the JSON they
// answer.
package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/billing"
	"proxima.la/sello/sign/internal/cache"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/ratelimit"
	"proxima.la/sello/sign/internal/store"
)

//go:generate cp ../../../../contracts/openapi.yaml openapi.yaml
//go:embed openapi.yaml
var openapiYAML []byte

// Deps are what the server needs.
type Deps struct {
	Store *store.Store
	Keys  cache.Keys
	// Limiter counts the signing rate; in memory if nil.
	Limiter  ratelimit.Limiter
	Signer   *Signer
	Verifier auth.TokenVerifier
	// Cloud is sign.sello.tax: spaces of Sello's IdP, billing and claims.
	Cloud bool
	// PlatformWorkspace is the space of Sello's platform (cloud).
	PlatformWorkspace string
	// Billing calls Sello's billing; nil without it.
	Billing *billing.Client
	// BillingPushToken is the bearer billing pushes with.
	BillingPushToken string
	CORSOrigins      []string
	Now              func() time.Time
}

// Server serves the API.
type Server struct {
	Deps
	mux *http.ServeMux
	// mcp is the MCP transport of POST /mcp (mcp.go).
	mcp http.Handler

	touchMu sync.Mutex
	touched map[string]time.Time
}

// New builds the server.
func New(d Deps) *Server {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Keys == nil {
		d.Keys = cache.NewMemoryKeys()
	}
	if d.Limiter == nil {
		d.Limiter = ratelimit.NewMemory()
	}
	s := &Server{Deps: d, mux: http.NewServeMux(), touched: map[string]time.Time{}}
	s.mcp = s.newMCPHandler()
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /health", s.health)
	m.HandleFunc("GET /openapi.yaml", s.openapi)

	m.HandleFunc("POST /signatures", s.authed(s.signWithKey))
	m.HandleFunc("POST /verifications", s.authed(s.verify))
	m.HandleFunc("POST /mcp", s.serveMCP)
	m.HandleFunc("GET /mcp", mcpOnlyPost)
	m.HandleFunc("DELETE /mcp", mcpOnlyPost)

	m.HandleFunc("GET /me", s.authed(s.me))
	m.HandleFunc("GET /me/sello-organizations", s.authed(s.selloOrganizations))
	m.HandleFunc("GET /me/account-deletion", s.authed(s.accountDeletion))
	m.HandleFunc("DELETE /me", s.authed(s.deleteAccount))
	m.HandleFunc("POST /workspaces", s.authed(s.createWorkspace))
	m.HandleFunc("POST /invitation-lookups", s.authed(s.lookupInvitation))
	m.HandleFunc("POST /invitation-acceptances", s.authed(s.acceptInvitation))

	m.HandleFunc("GET /workspaces/{workspace_id}", s.inWorkspace(s.getWorkspace))
	m.HandleFunc("PATCH /workspaces/{workspace_id}", s.inWorkspace(s.updateWorkspace))
	m.HandleFunc("DELETE /workspaces/{workspace_id}", s.inWorkspace(s.deleteWorkspace))
	m.HandleFunc("GET /workspaces/{workspace_id}/members", s.inWorkspace(s.listMembers))
	m.HandleFunc("GET /workspaces/{workspace_id}/export", s.inWorkspace(s.exportWorkspace))
	m.HandleFunc("PATCH /workspaces/{workspace_id}/members/{user_id}", s.inWorkspace(s.updateMember))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/members/{user_id}", s.inWorkspace(s.removeMember))
	m.HandleFunc("GET /workspaces/{workspace_id}/invitations", s.inWorkspace(s.listInvitations))
	m.HandleFunc("POST /workspaces/{workspace_id}/invitations", s.inWorkspace(s.createInvitation))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/invitations/{invitation_id}", s.inWorkspace(s.revokeInvitation))
	m.HandleFunc("POST /workspaces/{workspace_id}/invitations/{invitation_id}/renewals", s.inWorkspace(s.renewInvitation))
	m.HandleFunc("GET /workspaces/{workspace_id}/usage", s.inWorkspace(s.usage))

	m.HandleFunc("GET /workspaces/{workspace_id}/organizations", s.inWorkspace(s.listOrganizations))
	m.HandleFunc("POST /workspaces/{workspace_id}/organizations", s.inWorkspace(s.createOrganization))
	m.HandleFunc("GET /workspaces/{workspace_id}/organizations/{organization_id}", s.inOrganization(s.getOrganization))
	m.HandleFunc("PATCH /workspaces/{workspace_id}/organizations/{organization_id}", s.inOrganization(s.updateOrganization))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/organizations/{organization_id}", s.inOrganization(s.deleteOrganization))
	m.HandleFunc("POST /workspaces/{workspace_id}/organizations/{organization_id}/signatures", s.inOrganization(s.signFromPanel))

	m.HandleFunc("GET /workspaces/{workspace_id}/organizations/{organization_id}/certificates", s.inOrganization(s.listCertificates))
	m.HandleFunc("POST /workspaces/{workspace_id}/organizations/{organization_id}/certificates", s.inOrganization(s.uploadCertificate))
	m.HandleFunc("PATCH /workspaces/{workspace_id}/organizations/{organization_id}/certificates/{certificate_id}", s.inOrganization(s.updateCertificate))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/organizations/{organization_id}/certificates/{certificate_id}", s.inOrganization(s.deleteCertificate))

	m.HandleFunc("GET /workspaces/{workspace_id}/organizations/{organization_id}/delegations", s.inOrganization(s.listDelegations))
	m.HandleFunc("POST /workspaces/{workspace_id}/organizations/{organization_id}/delegations", s.inOrganization(s.createDelegation))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/organizations/{organization_id}/delegations/{delegation_id}", s.inOrganization(s.deleteDelegation))

	m.HandleFunc("GET /workspaces/{workspace_id}/claimable-organizations", s.inWorkspace(s.claimableOrganizations))
	m.HandleFunc("POST /workspaces/{workspace_id}/organization-claims", s.inWorkspace(s.claimOrganizations))

	m.HandleFunc("GET /workspaces/{workspace_id}/api-keys", s.inWorkspace(s.listAPIKeys))
	m.HandleFunc("POST /workspaces/{workspace_id}/api-keys", s.inWorkspace(s.createAPIKey))
	m.HandleFunc("DELETE /workspaces/{workspace_id}/api-keys/{api_key_id}", s.inWorkspace(s.revokeAPIKey))

	m.HandleFunc("GET /workspaces/{workspace_id}/signatures", s.inWorkspace(s.listSignatures))
	m.HandleFunc("GET /workspaces/{workspace_id}/signatures/{signature_id}", s.inWorkspace(s.getSignature))

	m.HandleFunc("PUT /api/billing/organizations/{sello_workspace_id}", s.billingPush)

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeProblem(w, errNotFound) })
}

// Handler is the server with its middleware: recovery, CORS and the
// request log.
func (s *Server) Handler() http.Handler {
	return s.logRequests(s.recoverPanics(s.cors(s.mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		writeProblem(w, problem(http.StatusServiceUnavailable, "unavailable", "La base de datos no responde."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(openapiYAML)
}

// statusWriter records the status for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// logRequests logs each request: its method, its route's pattern (never
// the path, which may carry a tax id), its status and how long it took.
// Never a header or a body.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/health" && sw.status == http.StatusOK {
			return
		}
		level := slog.LevelInfo
		if sw.status >= 500 {
			level = slog.LevelError
		}
		slog.LogAttrs(r.Context(), level, "request",
			slog.String("method", r.Method), slog.String("route", r.Pattern),
			slog.Int("status", sw.status), slog.Int64("duration_ms", time.Since(start).Milliseconds()))
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "route", r.Pattern, "panic", v, "stack", string(debug.Stack()))
				writeProblem(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors lets the panel (and CORS_ORIGINS) call the API from a browser.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(s.CORSOrigins, origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Expose-Headers", "Retry-After, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Organization-Id")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// internalError logs err and answers 500.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "internal error", "route", r.Pattern, "error", err)
	writeProblem(w, errInternal)
}

// maxJSON caps a JSON body; a certificate in base64 fits well.
const maxJSON = 2 << 20

// decode reads a JSON body into v, refusing unknown fields.
func decode(w http.ResponseWriter, r *http.Request, v any) *Problem {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return problem(http.StatusUnsupportedMediaType, "unsupported-content-type", "El cuerpo debe ser application/json.")
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return problem(http.StatusRequestEntityTooLarge, "payload-too-large", "El cuerpo es demasiado grande.")
		}
		return invalid("El cuerpo no es un JSON válido para esta ruta.")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return invalid("El cuerpo debe ser un solo objeto JSON.")
	}
	return nil
}

// page reads ?limit= and ?cursor=.
func page(r *http.Request) (store.Page, *Problem) {
	q := r.URL.Query()
	p := store.Page{Limit: store.DefaultLimit, Cursor: q.Get("cursor")}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > store.MaxLimit {
			return p, invalid("«limit» debe ser un número entre 1 y 100.")
		}
		p.Limit = n
	}
	if _, ok := store.DecodeCursor(p.Cursor); !ok {
		return p, invalid("«cursor» no es uno que haya dado esta lista.")
	}
	return p, nil
}

// pathID reads a path value that must be a UUID; "" if it isn't one.
func pathID(r *http.Request, name string) string {
	id, _ := ids.Canonical(r.PathValue(name))
	return id
}

// optionalString is a JSON field that can be absent, null or a string.
type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

// optionalInt is a JSON field that can be absent, null or a whole number.
type optionalInt struct {
	Set   bool
	Value *int
}

func (o *optionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	o.Value = &n
	return nil
}

// ptr returns the pointer to the trimmed string, nil if empty.
func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	t := strings.TrimSpace(*p)
	if t == "" {
		return nil
	}
	return &t
}

func utc(t time.Time) time.Time { return t.UTC() }

func utcp(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
