package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
)

// listTools lists the tools a key with scopes gets, over the SDK's
// in-memory transport.
func listTools(t *testing.T, s *Server, k *auth.Key) []*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.mcpServerFor(mcpCaller{raw: "sign_test_x", key: k}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.Tools
}

// toolNames are the tools' names, in the order tools/list gives them: by
// name.
func toolNames(tools []*mcp.Tool) []string {
	var out []string
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func TestMCPToolsFollowTheScopes(t *testing.T) {
	s := New(Deps{})
	read := listTools(t, s, &auth.Key{Environment: model.Test, Scopes: []model.Scope{model.ScopeOrganizationsRead}})
	if got := toolNames(read); !slices.Equal(got, []string{"get_organization", "get_workspace", "list_organizations"}) {
		t.Fatalf("organizations:read lists %v", got)
	}
	signOnly := listTools(t, s, &auth.Key{Environment: model.Prod, Scopes: []model.Scope{model.ScopeSign}})
	if got := toolNames(signOnly); !slices.Equal(got, []string{"get_workspace", "sign_document", "verify_signature"}) {
		t.Fatalf("sign lists %v", got)
	}

	all := listTools(t, s, &auth.Key{Environment: model.Test, Scopes: model.AllScopes})
	if len(all) != len(mcpTools) {
		t.Fatalf("every scope lists %d tools of %d", len(all), len(mcpTools))
	}
	var destructive []string
	for _, tool := range all {
		a := tool.Annotations
		if a == nil || a.Title == "" || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: annotations %+v", tool.Name, a)
			continue
		}
		if a.ReadOnlyHint {
			if a.DestructiveHint != nil && *a.DestructiveHint {
				t.Errorf("%s is read-only and destructive", tool.Name)
			}
			continue
		}
		// A write tool always says: the hint's default is destructive.
		if a.DestructiveHint == nil {
			t.Errorf("%s: no destructiveHint", tool.Name)
			continue
		}
		if *a.DestructiveHint {
			destructive = append(destructive, tool.Name)
		}
	}
	want := []string{"deactivate_certificate", "delete_certificate", "delete_delegation", "delete_organization", "revoke_api_key"}
	if !slices.Equal(destructive, want) {
		t.Fatalf("destructive tools %v, want %v", destructive, want)
	}
	// Uploading a certificate and creating a key stay out.
	for _, name := range toolNames(all) {
		if strings.Contains(name, "upload") || name == "create_api_key" {
			t.Fatalf("%s is a tool", name)
		}
	}
}

// Each tool calls a route the server serves, and with the scope that
// route requires, which the tool is listed by.
func TestMCPToolsCallServedRoutes(t *testing.T) {
	s := New(Deps{})
	args := map[string]any{
		"organization": "0614", "document": map[string]any{"id": "1"}, "signature": "x",
		"organization_id": "o", "certificate_id": "c", "delegation_id": "d", "api_key_id": "k", "signature_id": "s",
		"name": "n", "tax_id": "1", "country": "SV", "workspace_id": "w", "scopes": []string{"sign"},
	}
	for _, tool := range mcpTools {
		schema := tool.tool.InputSchema.(map[string]any)
		in := map[string]any{}
		for name := range schema["properties"].(schemaProps) {
			if v, ok := args[name]; ok {
				in[name] = v
			}
		}
		raw, _ := json.Marshal(in)
		c, err := tool.call(raw)
		if err != nil {
			t.Fatalf("%s: %v", tool.tool.Name, err)
		}
		req := httptest.NewRequest(c.method, c.path, nil)
		if _, pattern := s.mux.Handler(req); pattern == "/" || pattern == "" {
			t.Errorf("%s calls %s %s, which isn't a route", tool.tool.Name, c.method, c.path)
		}
	}
}

func TestMCPArguments(t *testing.T) {
	sign := mcpTools[0]
	if sign.tool.Name != "sign_document" {
		t.Fatal("sign_document isn't the first tool")
	}
	// A JSON object reaches the route byte for byte, keys in their order.
	c, err := sign.call(json.RawMessage(`{"organization":"o","document":{"z":1, "a":[1.0,2],"m":"é"}}`))
	if err != nil || string(c.body) != `{"z":1, "a":[1.0,2],"m":"é"}` || c.contentType != "application/json" || c.organization != "o" {
		t.Fatalf("object: %q %q %v", c.body, c.contentType, err)
	}
	c, err = sign.call(json.RawMessage(`{"organization":"o","document":"  <a>&amp;</a>"}`))
	if err != nil || string(c.body) != "  <a>&amp;</a>" || c.contentType != "application/xml" {
		t.Fatalf("xml: %q %q %v", c.body, c.contentType, err)
	}
	c, err = sign.call(json.RawMessage(`{"organization":"o","document":"{\"b\":1,\"a\":2}"}`))
	if err != nil || string(c.body) != `{"b":1,"a":2}` || c.contentType != "application/json" {
		t.Fatalf("json text: %q %q %v", c.body, c.contentType, err)
	}
	for _, bad := range []string{
		`{"organization":"o","document":42}`,
		`{"organization":"o"}`,
		`{"document":{}}`,
		`{"organization":"o","document":{},"extra":1}`,
		`[1]`,
	} {
		if _, err := sign.call(json.RawMessage(bad)); err == nil {
			t.Errorf("%s was taken", bad)
		}
	}
	// Optional fields only go when given; null clears the contact email.
	for _, tool := range mcpTools {
		if tool.tool.Name != "update_organization" {
			continue
		}
		c, err := tool.call(json.RawMessage(`{"organization_id":"o","contact_email":null}`))
		if err != nil || string(c.body) != `{"contact_email":null}` || c.method != "PATCH" {
			t.Fatalf("update: %q %v", c.body, err)
		}
	}
}

func TestMCPKeyOf(t *testing.T) {
	for _, tc := range []struct {
		header, value, want string
	}{
		{"X-API-Key", " sign_test_abc ", "sign_test_abc"},
		{"Authorization", "Bearer sign_prod_abc", "sign_prod_abc"},
		{"Authorization", "bearer  sign_test_abc", "sign_test_abc"},
		{"Authorization", "Bearer eyJhbGciOiJFZERTQSJ9.x.y", ""},
		{"Authorization", "Basic sign_test_abc", ""},
		{"", "", ""},
	} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		if got := mcpKeyOf(r); got != tc.want {
			t.Errorf("%s: %q = %q, want %q", tc.header, tc.value, got, tc.want)
		}
	}
}

func TestMCPResults(t *testing.T) {
	ok := &recorder{header: http.Header{}, status: 204}
	if r := mcpResultOf(ok); r.IsError || r.Content[0].(*mcp.TextContent).Text != "Done." {
		t.Fatalf("204 = %+v", r)
	}
	limited := &recorder{header: http.Header{"Retry-After": {"50"}}}
	writeProblem(limited, &Problem{Type: "rate-limited", Title: "Too Many Requests", Status: 429, Detail: "x", AppliesTo: "workspace", Limit: new(3)})
	r := mcpResultOf(limited)
	var p map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &p); err != nil || !r.IsError {
		t.Fatalf("429 = %+v %v", r, err)
	}
	if p["type"] != "rate-limited" || p["applies_to"] != "workspace" || p["limit"] != float64(3) || p["retry_after"] != float64(50) || p["status"] != float64(429) {
		t.Fatalf("429 = %v", p)
	}
	plain := &recorder{header: http.Header{}, status: 405}
	_, _ = plain.Write([]byte("Method Not Allowed"))
	if r := mcpResultOf(plain); !r.IsError || !strings.Contains(r.Content[0].(*mcp.TextContent).Text, `"detail":"Method Not Allowed"`) {
		t.Fatalf("405 = %+v", r.Content[0])
	}
}

func TestMCPInstructions(t *testing.T) {
	got := mcpInstructions(&auth.Key{Environment: model.Prod, Scopes: []model.Scope{model.ScopeSign, model.ScopeLogsRead}, OrganizationID: "org-1"})
	for _, want := range []string{"`prod` environment", "sign, logs:read", "`org-1`"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions lack %q:\n%s", want, got)
		}
	}
}
