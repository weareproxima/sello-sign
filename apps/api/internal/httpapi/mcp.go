package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
)

// The MCP server (POST /mcp): the API as Model Context Protocol tools, for
// an agent to sign, verify and look things up without building the HTTP
// calls. Streamable HTTP, stateless, like Sello Chat's: each POST brings the
// key and gets a server whose tools are the ones the key's scopes allow.
//
// There is one implementation: every tool is a call to its route, made in
// process with the caller's own key, through the same ServeMux. Who may do
// what, the organization's reach, the per-minute and monthly limits, the
// errors and the signing log are the routes', never a second copy.
//
// Two routes are left out on purpose, as in ../sign-old: uploading a
// certificate would pass the private key and its password through the
// model's context, and creating a key would hand it a new secret it could
// leak. They stay in the panel and the REST API. So do the routes only a
// person's token reaches (members, invitations, the space's settings).

// errMCPUnauthorized is the 401 of /mcp: it takes keys alone.
var errMCPUnauthorized = problem(http.StatusUnauthorized, "unauthorized",
	"El servidor MCP pide una clave de API válida: en X-API-Key o como Authorization: Bearer sign_….")

// mcpCallerKey keys the caller in the request's context, from serveMCP to
// the server of the request.
type mcpCallerKey struct{}

// mcpCaller is the key of an MCP request: as it came, for the routes, and
// as it resolved, for the tools it may see.
type mcpCaller struct {
	raw string
	key *auth.Key
}

// newMCPHandler is the Streamable HTTP transport of /mcp.
func (s *Server) newMCPHandler() http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		c, ok := r.Context().Value(mcpCallerKey{}).(mcpCaller)
		if !ok || c.key == nil {
			return nil
		}
		return s.mcpServerFor(c)
	}, &mcp.StreamableHTTPOptions{
		// Stateless: the tools never ask the client anything midway, which
		// is all a session would add, and nothing is kept between requests
		// or lost in a deployment. GET and DELETE are 405.
		Stateless: true,
		// Each answer is one JSON message: no tool streams.
		JSONResponse: true,
		// The API listens on a loopback address behind a proxy that passes
		// it api.sign.sello.tax: the SDK's DNS rebinding protection would
		// refuse every request. It protects a local server with access to
		// the machine; this one asks for a key on every request.
		DisableLocalhostProtection: true,
		// A document to sign travels inside the JSON-RPC message, maybe
		// escaped as a string; the route caps it again at maxDocument.
		MaxRequestBodyBytes: 2 * maxDocument,
	})
	// An MCP client isn't a web page: a browser's request from another
	// origin is refused, which is the Origin check the transport asks for.
	return http.NewCrossOriginProtection().Handler(h)
}

// serveMCP handles POST /mcp: it takes the key from X-API-Key or, for the
// clients that only send that, Authorization: Bearer, resolves it as every
// route does, and hands the request to the transport.
func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	raw := mcpKeyOf(r)
	if raw == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sello-sign"`)
		writeProblem(w, errMCPUnauthorized)
		return
	}
	r.Header.Set("X-API-Key", raw)
	p, prob := s.authenticate(r)
	if prob == errUnauthorized {
		prob = errMCPUnauthorized
	}
	if prob != nil {
		writeProblem(w, prob)
		return
	}
	ctx := context.WithValue(r.Context(), mcpCallerKey{}, mcpCaller{raw: raw, key: p.Key})
	s.mcp.ServeHTTP(w, r.WithContext(ctx))
}

// mcpOnlyPost handles GET and DELETE /mcp: without sessions there is no
// stream to open and none to end, and the transport asks for 405 (the
// catch-all route would answer 404).
func mcpOnlyPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	writeProblem(w, problem(http.StatusMethodNotAllowed, "method-not-allowed", "El servidor MCP no tiene sesiones: solo acepta POST."))
}

// mcpKeyOf is the key of an MCP request; "" without one (a panel token
// isn't one).
func mcpKeyOf(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("X-API-Key")); k != "" {
		return k
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	token = strings.TrimSpace(token)
	if ok && strings.EqualFold(scheme, "bearer") && strings.HasPrefix(token, auth.KeyPrefix) {
		return token
	}
	return ""
}

// mcpServerFor builds the server of one request: the tools c's scopes
// allow, each bound to c's key.
func (s *Server) mcpServerFor(c mcpCaller) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "sello-sign", Title: "Sello Sign", Version: serverVersion()},
		&mcp.ServerOptions{Instructions: mcpInstructions(c.key)})
	for _, t := range mcpTools {
		if t.scope == "" || model.HasScope(c.key.Scopes, t.scope) {
			srv.AddTool(t.tool, s.mcpHandler(t, c.raw))
		}
	}
	return srv
}

// mcpInstructions tell the model what Sign is and what this key may do.
func mcpInstructions(k *auth.Key) string {
	scopes := make([]string, len(k.Scopes))
	for i, sc := range k.Scopes {
		scopes[i] = string(sc)
	}
	lines := []string{
		"Sello Sign signs and verifies electronic invoicing documents with the method each country's tax authority requires: a JWS over the JSON DTE in El Salvador, XMLDSig or XAdES over the XML elsewhere. It does not send documents to the tax authority.",
		"A workspace is Sign's customer and keeps organizations: the taxpayers, each with its certificates. A delegation lets another workspace sign with an organization it does not own. Management tools take an organization's `id`; sign_document and verify_signature also take its tax id.",
		fmt.Sprintf("This API key works in the `%s` environment with the scopes %s; only the tools those scopes allow are listed.", k.Environment, strings.Join(scopes, ", ")),
		"Signing counts against the workspace's per-minute and monthly limits (get_workspace shows them). A failed call returns the API's problem as JSON: `type`, `detail` and, for a limit, `applies_to`, `limit` and `retry_after` in seconds. Texts from the API are in Spanish.",
	}
	if k.OrganizationID != "" {
		lines = append(lines, fmt.Sprintf("The key is limited to one organization, whose id is `%s`.", k.OrganizationID))
	}
	return strings.Join(lines, "\n")
}

// serverVersion is the module's version, or "dev".
func serverVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// mcpCall is the route a tool calls.
type mcpCall struct {
	method, path string
	query        url.Values
	body         []byte
	contentType  string
	// organization is X-Organization-Id.
	organization string
}

// mcpTool is a tool and the route it calls.
type mcpTool struct {
	// scope is what the key needs for the tool to be listed: the one its
	// route requires; "" for any key.
	scope model.Scope
	tool  *mcp.Tool
	call  func(args json.RawMessage) (mcpCall, error)
}

// mcpHandler runs t's route with key.
func (s *Server) mcpHandler(t mcpTool, key string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := t.call(req.Params.Arguments)
		if err != nil {
			return mcpError(map[string]any{"type": "invalid-arguments", "status": http.StatusBadRequest, "detail": err.Error()}), nil
		}
		return s.mcpDispatch(ctx, t.tool.Name, key, c), nil
	}
}

// mcpDispatch serves c through the routes, as a request with key would be
// served, and turns the answer into the tool's result. The request log
// says which tool called which route, never its arguments.
func (s *Server) mcpDispatch(ctx context.Context, tool, key string, c mcpCall) *mcp.CallToolResult {
	target := c.path
	if len(c.query) > 0 {
		target += "?" + c.query.Encode()
	}
	var body io.Reader = http.NoBody
	if c.body != nil {
		body = bytes.NewReader(c.body)
	}
	req, err := http.NewRequestWithContext(ctx, c.method, target, body)
	if err != nil {
		slog.ErrorContext(ctx, "mcp: could not build a request", "tool", tool, "error", err)
		return mcpError(map[string]any{"type": errInternal.Type, "status": errInternal.Status, "detail": errInternal.Detail})
	}
	req.Header.Set("X-API-Key", key)
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	if c.organization != "" {
		req.Header.Set("X-Organization-Id", c.organization)
	}
	rec := &recorder{header: http.Header{}}
	start := time.Now()
	s.recoverPanics(s.mux).ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	level := slog.LevelInfo
	if rec.status >= 500 {
		level = slog.LevelError
	}
	slog.LogAttrs(ctx, level, "mcp tool",
		slog.String("tool", tool), slog.String("route", req.Pattern),
		slog.Int("status", rec.status), slog.Int64("duration_ms", time.Since(start).Milliseconds()))
	return mcpResultOf(rec)
}

// mcpResultOf passes a success body through untouched (the JSON the REST
// API documents) and turns a problem into a tool error the model reads
// and can act on, rather than a protocol error it never sees. Retry-After
// goes in it as retry_after.
func mcpResultOf(rec *recorder) *mcp.CallToolResult {
	text := strings.TrimSpace(rec.body.String())
	if rec.status >= 200 && rec.status < 300 {
		if text == "" {
			text = "Done."
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
	}
	var p map[string]any
	if json.Unmarshal([]byte(text), &p) != nil || p == nil {
		p = map[string]any{"status": rec.status, "detail": text}
	}
	if n, err := strconv.Atoi(rec.header.Get("Retry-After")); err == nil {
		p["retry_after"] = n
	}
	return mcpError(p)
}

func mcpError(p map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(p)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

// recorder is the ResponseWriter of a tool's route.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}

// decodeArgs reads a tool's arguments into v, refusing unknown ones. The
// SDK leaves that to the tool: its typed tools would decode them into a
// map and write them again, and a document to sign must reach the route
// with its keys in their order (El Salvador's JWS signs them so).
func decodeArgs(raw json.RawMessage, v any) error {
	if t := bytes.TrimSpace(raw); len(t) == 0 || bytes.Equal(t, []byte("null")) {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid arguments: %v", err)
	}
	return nil
}

// required fails if any of the named values is empty.
func required(pairs ...string) error {
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			return fmt.Errorf("missing %q", pairs[i])
		}
	}
	return nil
}

func jsonBody(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// seg escapes a path segment.
func seg(s string) string { return url.PathEscape(s) }

// paging is the arguments of a paginated list.
type paging struct {
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

func (p paging) query() url.Values {
	q := url.Values{}
	if p.Limit != nil {
		q.Set("limit", strconv.Itoa(*p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	return q
}

// The tools' input schemas.

type schemaProps map[string]any

func object(props schemaProps, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func schemaString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func schemaEnum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

var (
	argOrganizationRef = schemaString("The organization: its `id` or its tax id, with or without separators (sent as X-Organization-Id).")
	argOrganizationID  = schemaString("The organization's `id`, as list_organizations gives it.")
	argCertificateID   = schemaString("The certificate's `id`, as list_certificates gives it.")
	argLimit           = map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Page size, 50 by default."}
	argCursor          = schemaString("The `next_cursor` of the previous page.")
)

func ptrBool(b bool) *bool { return &b }

// Annotations: closed world everywhere (Sign's own data); destructive on
// what deletes or stops something working, which clients confirm first.
var (
	readOnly    = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptrBool(false)}
	additive    = &mcp.ToolAnnotations{DestructiveHint: ptrBool(false), OpenWorldHint: ptrBool(false)}
	idempotent  = &mcp.ToolAnnotations{DestructiveHint: ptrBool(false), IdempotentHint: true, OpenWorldHint: ptrBool(false)}
	destructive = &mcp.ToolAnnotations{DestructiveHint: ptrBool(true), IdempotentHint: true, OpenWorldHint: ptrBool(false)}
)

func titled(a *mcp.ToolAnnotations, title string) *mcp.ToolAnnotations {
	c := *a
	c.Title = title
	return &c
}

// orgPath is the path of an organization of the key's space.
func orgPath(id string) string { return "/workspaces/self/organizations/" + seg(id) }

// mcpTools are the tools (tools/list gives them by name). Each names its
// route in the contract.
var mcpTools = []mcpTool{
	{
		scope: model.ScopeSign,
		tool: &mcp.Tool{
			Name: "sign_document",
			Description: "Signs an electronic invoicing document with the organization's active certificate in the key's environment, with its country's method (POST /signatures). " +
				"Pass a JSON document as an object (El Salvador) or an XML document as a string (the other countries); a string that is JSON text is signed as JSON. " +
				"Each call counts against the workspace's per-minute and monthly limits and is recorded in the signing log. Returns `signature` (a JWS in El Salvador, the signed XML elsewhere), `document_id` and `document_type`.",
			InputSchema: object(schemaProps{
				"organization": argOrganizationRef,
				"document": map[string]any{
					"type":        []string{"object", "string"},
					"description": "The document to sign: a JSON object, or the document as text (XML or JSON).",
				},
			}, "organization", "document"),
			Annotations: titled(additive, "Sign a document"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				Organization string          `json:"organization"`
				Document     json.RawMessage `json:"document"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("organization", a.Organization); err != nil {
				return mcpCall{}, err
			}
			body, ct, err := documentOf(a.Document)
			if err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "POST", path: "/signatures", organization: a.Organization, body: body, contentType: ct}, nil
		},
	},
	{
		scope: model.ScopeSign,
		tool: &mcp.Tool{
			Name: "verify_signature",
			Description: "Verifies a signature against the organization's active certificate in the key's environment (POST /verifications). " +
				"Returns `valid`, the signed `document` as text, `document_id` and `document_type`.",
			InputSchema: object(schemaProps{
				"organization": argOrganizationRef,
				"signature":    schemaString("A JWS (El Salvador) or a signed XML document (the other countries)."),
			}, "organization", "signature"),
			Annotations: titled(readOnly, "Verify a signature"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				Organization string `json:"organization"`
				Signature    string `json:"signature"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("organization", a.Organization, "signature", a.Signature); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "POST", path: "/verifications", organization: a.Organization,
				body: jsonBody(map[string]string{"signature": a.Signature}), contentType: "application/json"}, nil
		},
	},
	{
		tool: &mcp.Tool{
			Name:        "get_workspace",
			Description: "Returns the key's workspace (GET /workspaces/self): its name, plan, whether it is closed, and its limits: signatures a minute, a month per environment, organizations and the signing log's retention.",
			InputSchema: object(schemaProps{}),
			Annotations: titled(readOnly, "Get the workspace"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct{}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "GET", path: "/workspaces/self"}, nil
		},
	},
	{
		scope: model.ScopeOrganizationsRead,
		tool: &mcp.Tool{
			Name: "list_organizations",
			Description: "Lists the organizations the key reaches, newest first (GET /workspaces/self/organizations): the workspace's own and those delegated to it, " +
				"each with its `id`, `name`, `tax_id`, `country`, `relation`, its delegation if delegated, and its active certificate per environment.",
			InputSchema: object(schemaProps{
				"relation": schemaEnum("Only the workspace's own (owned) or only the delegated ones.", "owned", "delegated"),
				"limit":    argLimit,
				"cursor":   argCursor,
			}),
			Annotations: titled(readOnly, "List organizations"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				Relation string `json:"relation"`
				paging
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			q := a.query()
			if a.Relation != "" {
				q.Set("relation", a.Relation)
			}
			return mcpCall{method: "GET", path: "/workspaces/self/organizations", query: q}, nil
		},
	},
	{
		scope: model.ScopeOrganizationsRead,
		tool: &mcp.Tool{
			Name:        "get_organization",
			Description: "Returns one organization (GET /workspaces/self/organizations/{id}), with its active certificate per environment.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID}, "organization_id"),
			Annotations: titled(readOnly, "Get an organization"),
		},
		call: organizationCall("GET", ""),
	},
	{
		scope: model.ScopeOrganizationsWrite,
		tool: &mcp.Tool{
			Name: "create_organization",
			Description: "Creates an organization in the workspace (POST /workspaces/self/organizations). Idempotent by country and tax id: if it exists, returns it. " +
				"Subject to the plan's organization limit. Its certificates are uploaded afterwards, from the panel or the REST API.",
			InputSchema: object(schemaProps{
				"name":          schemaString("The taxpayer's name."),
				"tax_id":        schemaString("Its tax id (in El Salvador, the NIT), with or without separators."),
				"country":       schemaString("Its country, ISO 3166-1 alpha-2 (SV, CR, DO, PA, GT…): it decides how its documents are signed. Never changes."),
				"contact_email": schemaString("An email to reach it, optional."),
			}, "name", "tax_id", "country"),
			Annotations: titled(idempotent, "Create an organization"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				Name         string `json:"name"`
				TaxID        string `json:"tax_id"`
				Country      string `json:"country"`
				ContactEmail string `json:"contact_email"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("name", a.Name, "tax_id", a.TaxID, "country", a.Country); err != nil {
				return mcpCall{}, err
			}
			body := map[string]string{"name": a.Name, "tax_id": a.TaxID, "country": a.Country}
			if a.ContactEmail != "" {
				body["contact_email"] = a.ContactEmail
			}
			return mcpCall{method: "POST", path: "/workspaces/self/organizations", body: jsonBody(body), contentType: "application/json"}, nil
		},
	},
	{
		scope: model.ScopeOrganizationsWrite,
		tool: &mcp.Tool{
			Name:        "update_organization",
			Description: "Changes an organization's name or contact email (PATCH /workspaces/self/organizations/{id}). Its country and tax id never change. Only its owner workspace may.",
			InputSchema: object(schemaProps{
				"organization_id": argOrganizationID,
				"name":            schemaString("The new name."),
				"contact_email":   map[string]any{"type": []string{"string", "null"}, "description": "The new contact email; null clears it."},
			}, "organization_id"),
			Annotations: titled(idempotent, "Update an organization"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				OrganizationID string          `json:"organization_id"`
				Name           *string         `json:"name"`
				ContactEmail   json.RawMessage `json:"contact_email"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("organization_id", a.OrganizationID); err != nil {
				return mcpCall{}, err
			}
			body := map[string]any{}
			if a.Name != nil {
				body["name"] = *a.Name
			}
			if a.ContactEmail != nil {
				body["contact_email"] = a.ContactEmail
			}
			return mcpCall{method: "PATCH", path: orgPath(a.OrganizationID), body: jsonBody(body), contentType: "application/json"}, nil
		},
	},
	{
		scope: model.ScopeOrganizationsWrite,
		tool: &mcp.Tool{
			Name: "delete_organization",
			Description: "Deletes an organization with its certificates (DELETE /workspaces/self/organizations/{id}). Its signing log stays, without its name. " +
				"Refused while it has delegations. Cannot be undone.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID}, "organization_id"),
			Annotations: titled(destructive, "Delete an organization"),
		},
		call: organizationCall("DELETE", ""),
	},
	{
		scope: model.ScopeCertificatesRead,
		tool: &mcp.Tool{
			Name:        "list_certificates",
			Description: "Lists an organization's certificates (GET …/organizations/{id}/certificates): environment, tax id, subject, issuer, serial, validity and which one is active in each environment. Never the certificate's key.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID}, "organization_id"),
			Annotations: titled(readOnly, "List certificates"),
		},
		call: organizationCall("GET", "/certificates"),
	},
	{
		scope: model.ScopeCertificatesWrite,
		tool: &mcp.Tool{
			Name:        "activate_certificate",
			Description: "Makes a certificate the one that signs in its environment (PATCH …/certificates/{id}, active: true); the one active before is deactivated.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID, "certificate_id": argCertificateID}, "organization_id", "certificate_id"),
			Annotations: titled(idempotent, "Activate a certificate"),
		},
		call: certificateCall("PATCH", map[string]bool{"active": true}),
	},
	{
		scope: model.ScopeCertificatesWrite,
		tool: &mcp.Tool{
			Name: "deactivate_certificate",
			Description: "Deactivates a certificate (PATCH …/certificates/{id}, active: false). If it was the active one, the organization stops signing in that environment " +
				"until another is activated.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID, "certificate_id": argCertificateID}, "organization_id", "certificate_id"),
			Annotations: titled(destructive, "Deactivate a certificate"),
		},
		call: certificateCall("PATCH", map[string]bool{"active": false}),
	},
	{
		scope: model.ScopeCertificatesWrite,
		tool: &mcp.Tool{
			Name:        "delete_certificate",
			Description: "Deletes a certificate and its key (DELETE …/certificates/{id}). If it was the active one, the organization stops signing in that environment. Cannot be undone: it must be uploaded again.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID, "certificate_id": argCertificateID}, "organization_id", "certificate_id"),
			Annotations: titled(destructive, "Delete a certificate"),
		},
		call: certificateCall("DELETE", nil),
	},
	{
		scope: model.ScopeDelegationsRead,
		tool: &mcp.Tool{
			Name:        "list_delegations",
			Description: "Lists the delegations of an organization the workspace owns (GET …/organizations/{id}/delegations): the workspaces that may sign with it, their scopes and whether the delegation is locked.",
			InputSchema: object(schemaProps{"organization_id": argOrganizationID}, "organization_id"),
			Annotations: titled(readOnly, "List delegations"),
		},
		call: organizationCall("GET", "/delegations"),
	},
	{
		scope: model.ScopeDelegationsWrite,
		tool: &mcp.Tool{
			Name:        "create_delegation",
			Description: "Lets another workspace sign with an organization the workspace owns, and maybe read or change its certificates (POST …/organizations/{id}/delegations).",
			InputSchema: object(schemaProps{
				"organization_id": argOrganizationID,
				"workspace_id":    schemaString("The `id` of the workspace that receives it."),
				"scopes": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"items":       map[string]any{"type": "string", "enum": []string{"sign", "certificates:read", "certificates:write"}},
					"description": "What it may do with the organization.",
				},
			}, "organization_id", "workspace_id", "scopes"),
			Annotations: titled(additive, "Create a delegation"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				OrganizationID string   `json:"organization_id"`
				WorkspaceID    string   `json:"workspace_id"`
				Scopes         []string `json:"scopes"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("organization_id", a.OrganizationID, "workspace_id", a.WorkspaceID); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "POST", path: orgPath(a.OrganizationID) + "/delegations",
				body: jsonBody(map[string]any{"workspace_id": a.WorkspaceID, "scopes": a.Scopes}), contentType: "application/json"}, nil
		},
	},
	{
		scope: model.ScopeDelegationsWrite,
		tool: &mcp.Tool{
			Name: "delete_delegation",
			Description: "Removes a delegation (DELETE …/organizations/{id}/delegations/{delegation_id}): the other workspace stops signing with the organization at once. " +
				"The workspace that received it may also give it up, under its own path. A locked one can only be given up.",
			InputSchema: object(schemaProps{
				"organization_id": argOrganizationID,
				"delegation_id":   schemaString("The delegation's `id`, as list_delegations or list_organizations give it."),
			}, "organization_id", "delegation_id"),
			Annotations: titled(destructive, "Remove a delegation"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				OrganizationID string `json:"organization_id"`
				DelegationID   string `json:"delegation_id"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("organization_id", a.OrganizationID, "delegation_id", a.DelegationID); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "DELETE", path: orgPath(a.OrganizationID) + "/delegations/" + seg(a.DelegationID)}, nil
		},
	},
	{
		scope: model.ScopeKeysRead,
		tool: &mcp.Tool{
			Name:        "list_api_keys",
			Description: "Lists the workspace's API keys, newest first (GET /workspaces/self/api-keys): name, prefix, environment, scopes, organization, rate and last use. The keys themselves are never returned.",
			InputSchema: object(schemaProps{"limit": argLimit, "cursor": argCursor}),
			Annotations: titled(readOnly, "List API keys"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a paging
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "GET", path: "/workspaces/self/api-keys", query: a.query()}, nil
		},
	},
	{
		scope: model.ScopeKeysWrite,
		tool: &mcp.Tool{
			Name:        "revoke_api_key",
			Description: "Revokes an API key at once (DELETE /workspaces/self/api-keys/{id}): whatever uses it stops working. Cannot be undone.",
			InputSchema: object(schemaProps{"api_key_id": schemaString("The key's `id`, as list_api_keys gives it.")}, "api_key_id"),
			Annotations: titled(destructive, "Revoke an API key"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				APIKeyID string `json:"api_key_id"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("api_key_id", a.APIKeyID); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "DELETE", path: "/workspaces/self/api-keys/" + seg(a.APIKeyID)}, nil
		},
	},
	{
		scope: model.ScopeLogsRead,
		tool: &mcp.Tool{
			Name: "list_signatures",
			Description: "Lists the signing log, newest first (GET /workspaces/self/signatures): organization, environment, document id and type, the key or person that signed, " +
				"the workspace that signed (another one, by delegation) and how long it took. Never the document. Entries last the signing workspace's retention.",
			InputSchema: object(schemaProps{
				"organization_id": argOrganizationID,
				"environment":     schemaEnum("Only this environment.", "test", "prod"),
				"from":            schemaString("From this instant, RFC 3339 (2026-11-01T00:00:00Z)."),
				"to":              schemaString("Up to this instant, RFC 3339."),
				"limit":           argLimit,
				"cursor":          argCursor,
			}),
			Annotations: titled(readOnly, "List signatures"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				OrganizationID string `json:"organization_id"`
				Environment    string `json:"environment"`
				From           string `json:"from"`
				To             string `json:"to"`
				paging
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			q := a.query()
			for k, v := range map[string]string{"organization_id": a.OrganizationID, "environment": a.Environment, "from": a.From, "to": a.To} {
				if v != "" {
					q.Set(k, v)
				}
			}
			return mcpCall{method: "GET", path: "/workspaces/self/signatures", query: q}, nil
		},
	},
	{
		scope: model.ScopeLogsRead,
		tool: &mcp.Tool{
			Name:        "get_signature",
			Description: "Returns one entry of the signing log (GET /workspaces/self/signatures/{id}).",
			InputSchema: object(schemaProps{"signature_id": schemaString("The entry's `id`, as list_signatures gives it.")}, "signature_id"),
			Annotations: titled(readOnly, "Get a signature"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				SignatureID string `json:"signature_id"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			if err := required("signature_id", a.SignatureID); err != nil {
				return mcpCall{}, err
			}
			return mcpCall{method: "GET", path: "/workspaces/self/signatures/" + seg(a.SignatureID)}, nil
		},
	},
	{
		scope: model.ScopeLogsRead,
		tool: &mcp.Tool{
			Name:        "get_usage",
			Description: "Returns what the workspace signed, per month and environment (GET /workspaces/self/usage), the last twelve months by default, with its plan and limits.",
			InputSchema: object(schemaProps{
				"from": schemaString("The first month, YYYY-MM."),
				"to":   schemaString("The last month, YYYY-MM."),
			}),
			Annotations: titled(readOnly, "Get the usage"),
		},
		call: func(raw json.RawMessage) (mcpCall, error) {
			var a struct {
				From string `json:"from"`
				To   string `json:"to"`
			}
			if err := decodeArgs(raw, &a); err != nil {
				return mcpCall{}, err
			}
			q := url.Values{}
			if a.From != "" {
				q.Set("from", a.From)
			}
			if a.To != "" {
				q.Set("to", a.To)
			}
			return mcpCall{method: "GET", path: "/workspaces/self/usage", query: q}, nil
		},
	},
}

// organizationCall is a tool whose only argument is organization_id: the
// route is method on the organization's path plus suffix.
func organizationCall(method, suffix string) func(json.RawMessage) (mcpCall, error) {
	return func(raw json.RawMessage) (mcpCall, error) {
		var a struct {
			OrganizationID string `json:"organization_id"`
		}
		if err := decodeArgs(raw, &a); err != nil {
			return mcpCall{}, err
		}
		if err := required("organization_id", a.OrganizationID); err != nil {
			return mcpCall{}, err
		}
		return mcpCall{method: method, path: orgPath(a.OrganizationID) + suffix}, nil
	}
}

// certificateCall is a tool on one certificate, with body if not nil.
func certificateCall(method string, body map[string]bool) func(json.RawMessage) (mcpCall, error) {
	return func(raw json.RawMessage) (mcpCall, error) {
		var a struct {
			OrganizationID string `json:"organization_id"`
			CertificateID  string `json:"certificate_id"`
		}
		if err := decodeArgs(raw, &a); err != nil {
			return mcpCall{}, err
		}
		if err := required("organization_id", a.OrganizationID, "certificate_id", a.CertificateID); err != nil {
			return mcpCall{}, err
		}
		c := mcpCall{method: method, path: orgPath(a.OrganizationID) + "/certificates/" + seg(a.CertificateID)}
		if body != nil {
			c.body, c.contentType = jsonBody(body), "application/json"
		}
		return c, nil
	}
}

// documentOf is the body and media type of a document to sign: a JSON
// object as it came, byte for byte; a string as its text, XML if it
// starts with «<» and JSON otherwise.
func documentOf(raw json.RawMessage) ([]byte, string, error) {
	t := bytes.TrimSpace(raw)
	switch {
	case len(t) == 0 || bytes.Equal(t, []byte("null")):
		return nil, "", errors.New(`missing "document"`)
	case t[0] == '{':
		return t, "application/json", nil
	case t[0] == '"':
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return nil, "", fmt.Errorf("invalid arguments: %v", err)
		}
		if strings.HasPrefix(strings.TrimSpace(s), "<") {
			return []byte(s), "application/xml", nil
		}
		return []byte(s), "application/json", nil
	}
	return nil, "", errors.New(`"document" must be a JSON object or a string`)
}
