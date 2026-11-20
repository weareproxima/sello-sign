package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/store"
)

// rpc sends one JSON-RPC message to /mcp with key, as a Streamable HTTP
// client does, and returns the response.
func (h *harness) rpc(key string, body string, headers ...string) response {
	h.t.Helper()
	all := append([]string{"Accept", "application/json, text/event-stream", "MCP-Protocol-Version", "2025-06-18"}, headers...)
	return h.do("POST", "/mcp", key, body, all...)
}

// result is the JSON-RPC result of r, failing on a JSON-RPC error.
func (r response) result(t *testing.T) map[string]any {
	t.Helper()
	m := r.json(t)
	if m["error"] != nil || m["result"] == nil {
		t.Fatalf("JSON-RPC error (%d): %s", r.status, r.body)
	}
	return m["result"].(map[string]any)
}

// toolText is a tools/call result's text and whether it is an error.
func toolText(t *testing.T, res map[string]any) (string, bool) {
	t.Helper()
	content := res["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v", content)
	}
	isError, _ := res["isError"].(bool)
	return content[0].(map[string]any)["text"].(string), isError
}

func callTool(name, args string) string {
	return `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

// A whole MCP conversation over HTTP: initialize, the tools of the key's
// scopes, a signature that is the route's (the same document, the same
// log) and the route's per-minute limit as a tool error.
func TestMCPConversation(t *testing.T) {
	clk := &clock{now: time.Date(2026, 11, 20, 10, 0, 10, 0, time.UTC)}
	h := newHarness(t, func(d *Deps) { d.Now = clk.Now })
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "Proveedor")
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	h.certificate(ana, ws, org, "test", "06141234561012")
	key := h.key(ana, ws, "test", []string{"sign", "logs:read"}, "")

	// Credentials: a key, never the panel's token.
	h.expect(h.rpc("", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), 401, "unauthorized")
	h.expect(h.rpc(ana, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), 401, "unauthorized")
	h.expect(h.rpc("sign_test_nope", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), 401, "unauthorized")
	if r := h.do("GET", "/mcp", key, nil); r.status != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp = %d", r.status)
	}
	// A browser on another origin.
	h.expect(h.rpc(key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "Origin", "https://evil.example", "Sec-Fetch-Site", "cross-site"), 403)

	// initialize, with the key in Authorization as some clients send it.
	init := h.rpc("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		"Authorization", "Bearer "+key)
	if ct := init.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("initialize answered %q", ct)
	}
	res := init.result(t)
	if res["protocolVersion"] != "2025-06-18" || res["serverInfo"].(map[string]any)["name"] != "sello-sign" ||
		!strings.Contains(res["instructions"].(string), "sign, logs:read") {
		t.Fatalf("initialize = %v", res)
	}
	h.expect(h.rpc(key, `{"jsonrpc":"2.0","method":"notifications/initialized"}`), 202)

	// tools/list: what sign and logs:read allow.
	var names []string
	for _, tool := range h.rpc(key, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`).result(t)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	if want := []string{"get_signature", "get_usage", "get_workspace", "list_signatures", "sign_document", "verify_signature"}; !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}

	// A signature: the document reaches the signer byte for byte.
	doc := `{"type":"01","id":"d1","n":1.50}`
	text, isErr := toolText(t, h.rpc(key, callTool("sign_document", `{"organization":"0614 123456 101 2","document":`+doc+`}`)).result(t))
	var signed map[string]string
	if err := json.Unmarshal([]byte(text), &signed); err != nil || isErr || signed["document_id"] != "d1" || signed["document_type"] != "01" {
		t.Fatalf("sign_document = %s (error %v)", text, isErr)
	}
	b64, _, _ := strings.Cut(signed["signature"], ".")
	if body, _ := base64.RawURLEncoding.DecodeString(b64); string(body) != doc {
		t.Fatalf("signed %q, sent %q", body, doc)
	}
	text, isErr = toolText(t, h.rpc(key, callTool("verify_signature", `{"organization":"`+org+`","signature":"`+signed["signature"]+`"}`)).result(t))
	if isErr || !strings.Contains(text, `"valid":true`) {
		t.Fatalf("verify_signature = %s", text)
	}
	// It is in the log with its key, as one made with the route.
	log := h.expect(h.do("GET", "/workspaces/"+ws+"/signatures", ana, nil), 200).items(t)
	keys := h.expect(h.do("GET", "/workspaces/"+ws+"/api-keys", ana, nil), 200).items(t)
	if len(log) != 1 || log[0]["document_id"] != "d1" || log[0]["api_key_id"] != keys[0]["id"] {
		t.Fatalf("log = %v", log)
	}
	text, _ = toolText(t, h.rpc(key, callTool("list_signatures", `{"environment":"test","limit":10}`)).result(t))
	if !strings.Contains(text, `"document_id":"d1"`) {
		t.Fatalf("list_signatures = %s", text)
	}

	// The route's errors, as tool errors.
	text, isErr = toolText(t, h.rpc(key, callTool("sign_document", `{"organization":"nope","document":{"id":"x"}}`)).result(t))
	if !isErr || !strings.Contains(text, `"type":"organization-not-found"`) {
		t.Fatalf("unknown organization = %s", text)
	}
	text, isErr = toolText(t, h.rpc(key, callTool("sign_document", `{"organization":"`+org+`"}`)).result(t))
	if !isErr || !strings.Contains(text, `"type":"invalid-arguments"`) {
		t.Fatalf("no document = %s", text)
	}
	// A tool the key's scopes don't list isn't there.
	if m := h.rpc(key, callTool("delete_organization", `{"organization_id":"`+org+`"}`)).json(t); m["error"] == nil {
		if _, isErr := toolText(t, m["result"].(map[string]any)); !isErr {
			t.Fatalf("delete_organization ran: %v", m)
		}
	}
	h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+org, ana, nil), 200)

	// The space's per-minute limit, the route's own.
	setLimits(t, h.db, ws, store.LimitsPatch{RequestsPerMinute: limit(1)})
	clk.add(time.Minute)
	sign := callTool("sign_document", `{"organization":"`+org+`","document":"{\"id\":\"d2\"}"}`)
	if _, isErr := toolText(t, h.rpc(key, sign).result(t)); isErr {
		t.Fatal("the first signature of the minute failed")
	}
	text, isErr = toolText(t, h.rpc(key, sign).result(t))
	var p map[string]any
	_ = json.Unmarshal([]byte(text), &p)
	if !isErr || p["type"] != "rate-limited" || p["applies_to"] != "workspace" || p["limit"] != float64(1) || p["retry_after"] != float64(50) || p["status"] != float64(429) {
		t.Fatalf("over the limit = %s", text)
	}
	if n := len(h.expect(h.do("GET", "/workspaces/"+ws+"/signatures", ana, nil), 200).items(t)); n != 2 {
		t.Fatalf("the log has %d signatures, want 2", n)
	}
}

// The management tools, with a key of every scope and with a key of one
// organization, through the SDK's own client.
func TestMCPManagement(t *testing.T) {
	h := newHarness(t, nil)
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "Proveedor")
	other := h.space(ana, "Otro proveedor")
	key := h.key(ana, ws, "test", []string{"sign", "organizations:read", "organizations:write", "certificates:read", "certificates:write",
		"delegations:read", "delegations:write", "keys:read", "keys:write", "logs:read"}, "")

	ctx := context.Background()
	connect := func(k string) *mcp.ClientSession {
		t.Helper()
		transport := &mcp.StreamableClientTransport{Endpoint: h.http.URL + "/mcp", HTTPClient: &http.Client{Transport: keyed{k}}}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, transport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cs.Close() })
		return cs
	}
	call := func(cs *mcp.ClientSession, name string, args map[string]any) (map[string]any, bool) {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		var m map[string]any
		if strings.HasPrefix(text, "{") {
			if err := json.Unmarshal([]byte(text), &m); err != nil {
				t.Fatalf("%s: %s", name, text)
			}
		}
		return m, res.IsError
	}
	cs := connect(key)
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != len(mcpTools) {
		t.Fatalf("tools: %d %v", len(tools.Tools), err)
	}

	created, isErr := call(cs, "create_organization", map[string]any{"name": "Empresa", "tax_id": "0614-1", "country": "SV"})
	if isErr || created["relation"] != "owned" {
		t.Fatalf("create_organization = %v", created)
	}
	org := created["id"].(string)
	if again, _ := call(cs, "create_organization", map[string]any{"name": "Otra", "tax_id": "06141", "country": "SV"}); again["id"] != org {
		t.Fatalf("not idempotent: %v", again)
	}
	if got, _ := call(cs, "update_organization", map[string]any{"organization_id": org, "contact_email": "a@b.sv"}); got["contact_email"] != "a@b.sv" {
		t.Fatalf("update_organization = %v", got)
	}
	if got, _ := call(cs, "list_organizations", map[string]any{"relation": "owned"}); len(got["items"].([]any)) != 1 {
		t.Fatalf("list_organizations = %v", got)
	}

	// Certificates: uploaded with the REST API, managed with the tools.
	cert := h.certificate(ana, ws, org, "test", "06141")
	if got, _ := call(cs, "list_certificates", map[string]any{"organization_id": org}); len(got["items"].([]any)) != 1 {
		t.Fatalf("list_certificates = %v", got)
	}
	if got, _ := call(cs, "deactivate_certificate", map[string]any{"organization_id": org, "certificate_id": cert}); got["active"] != false {
		t.Fatalf("deactivate_certificate = %v", got)
	}
	if got, isErr := call(cs, "sign_document", map[string]any{"organization": org, "document": map[string]any{"id": "x"}}); !isErr || got["type"] != "certificate-not-active" {
		t.Fatalf("signing without an active certificate = %v", got)
	}
	if got, _ := call(cs, "activate_certificate", map[string]any{"organization_id": org, "certificate_id": cert}); got["active"] != true {
		t.Fatalf("activate_certificate = %v", got)
	}

	// Delegations.
	d, isErr := call(cs, "create_delegation", map[string]any{"organization_id": org, "workspace_id": other, "scopes": []string{"sign"}})
	if isErr {
		t.Fatalf("create_delegation = %v", d)
	}
	if got, isErr := call(cs, "delete_organization", map[string]any{"organization_id": org}); !isErr || got["type"] != "organization-has-delegations" {
		t.Fatalf("delete with delegations = %v", got)
	}
	if got, _ := call(cs, "list_delegations", map[string]any{"organization_id": org}); len(got["items"].([]any)) != 1 {
		t.Fatalf("list_delegations = %v", got)
	}
	if _, isErr := call(cs, "delete_delegation", map[string]any{"organization_id": org, "delegation_id": d["id"]}); isErr {
		t.Fatal("delete_delegation failed")
	}

	// A key of one organization sees only it.
	only := h.key(ana, ws, "test", []string{"organizations:read", "keys:write"}, org)
	second := h.organization(ana, ws, "Segunda", "0614-2", "SV")
	oc := connect(only)
	if got, isErr := call(oc, "get_organization", map[string]any{"organization_id": second}); !isErr || got["type"] != "organization-not-found" {
		t.Fatalf("another organization = %v", got)
	}
	if got, _ := call(oc, "get_workspace", nil); got["id"] != ws {
		t.Fatalf("get_workspace = %v", got)
	}

	// Revoking a key stops it at once, also over MCP.
	keys, _ := call(cs, "list_api_keys", map[string]any{"limit": 1})
	newest := keys["items"].([]any)[0].(map[string]any)
	if newest["organization_id"] != org {
		t.Fatalf("newest key = %v", newest)
	}
	if got, isErr := call(cs, "revoke_api_key", map[string]any{"api_key_id": newest["id"]}); isErr {
		t.Fatalf("revoke_api_key = %v", got)
	}
	h.expect(h.rpc(only, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), 401, "unauthorized")

	if usage, _ := call(cs, "get_usage", map[string]any{"from": "2026-01"}); usage["items"] == nil {
		t.Fatalf("get_usage = %v", usage)
	}
	if _, isErr := call(cs, "delete_certificate", map[string]any{"organization_id": org, "certificate_id": cert}); isErr {
		t.Fatal("delete_certificate failed")
	}
	if _, isErr := call(cs, "delete_organization", map[string]any{"organization_id": org}); isErr {
		t.Fatal("delete_organization failed")
	}
}

// keyed sends the key with every request of the SDK's client.
type keyed struct{ key string }

func (k keyed) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-API-Key", k.key)
	return http.DefaultTransport.RoundTrip(r)
}
