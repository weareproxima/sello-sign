package httpapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
)

func fakeCert(taxID string) string { return base64.StdEncoding.EncodeToString([]byte("fake:" + taxID)) }

// space creates a space owned by the person of token and returns its id.
func (h *harness) space(token, name string) string {
	h.t.Helper()
	r := h.expect(h.do("POST", "/workspaces", token, map[string]any{"name": name, "default_country": "sv"}), 201)
	return str(r.json(h.t), "id")
}

func (h *harness) organization(token, ws, name, taxID, country string) string {
	h.t.Helper()
	r := h.do("POST", "/workspaces/"+ws+"/organizations", token, map[string]any{"name": name, "tax_id": taxID, "country": country})
	if r.status != 201 && r.status != 200 {
		h.t.Fatalf("organization: %d %s", r.status, r.body)
	}
	return str(r.json(h.t), "id")
}

func (h *harness) certificate(token, ws, org, env, taxID string) string {
	h.t.Helper()
	r := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/certificates", token,
		map[string]any{"environment": env, "file": fakeCert(taxID)}), 201)
	return str(r.json(h.t), "id")
}

func (h *harness) key(token, ws, env string, scopes []string, org string) string {
	h.t.Helper()
	body := map[string]any{"name": "k", "environment": env, "scopes": scopes}
	if org != "" {
		body["organization_id"] = org
	}
	r := h.expect(h.do("POST", "/workspaces/"+ws+"/api-keys", token, body), 201)
	return str(r.json(h.t), "key")
}

func TestSelfHostedFlow(t *testing.T) {
	h := newHarness(t, nil)
	ana := h.person("ana", auth.Claims{Name: "Ana"})

	h.expect(h.do("GET", "/me", "", nil), 401, "unauthorized")
	h.expect(h.do("GET", "/me", "bad-token", nil), 401, "unauthorized")
	ws := h.space(ana, "Proveedor A")
	me := h.expect(h.do("GET", "/me", ana, nil), 200).json(t)
	if spaces := me["workspaces"].([]any); len(spaces) != 1 || spaces[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("/me = %s", h.do("GET", "/me", ana, nil).body)
	}

	// Organizations: idempotent by country and tax id, whatever the separators.
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	again := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "Otra", "tax_id": "06141234561012", "country": "SV"}), 200).json(t)
	if str(again, "id") != org || str(again, "name") != "Empresa" {
		t.Fatalf("not idempotent: %v", again)
	}
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "X", "tax_id": "1", "country": "GT"}), 409, "country-not-enabled")
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "X", "tax_id": "1", "country": "SV", "sello_workspace_id": "org-x"}), 403, "forbidden")

	// Certificates.
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/certificates", ana,
		map[string]any{"environment": "test", "file": fakeCert("9999")}), 422, "certificate-tax-id-mismatch")
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/certificates", ana,
		map[string]any{"environment": "test", "file": base64.StdEncoding.EncodeToString([]byte("garbage"))}), 422, "invalid-certificate")
	first := h.certificate(ana, ws, org, "test", "06141234561012")
	second := h.certificate(ana, ws, org, "test", "0614-123456-101-2")
	certs := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+org+"/certificates", ana, nil), 200).items(t)
	if len(certs) != 2 || certs[0]["id"] != second || certs[0]["active"] != true || certs[1]["active"] != false {
		t.Fatalf("one active per environment: %v", certs)
	}
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/organizations/"+org+"/certificates/"+first, ana, map[string]any{"active": true}), 200)
	o := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+org, ana, nil), 200).json(t)
	if active := o["certificates"].(map[string]any)["test"].(map[string]any); active["id"] != first {
		t.Fatalf("active test certificate: %v", active)
	}
	if o["certificates"].(map[string]any)["prod"] != nil || o["relation"] != "owned" {
		t.Fatalf("organization: %v", o)
	}

	// Keys and signing.
	key := h.key(ana, ws, "test", []string{"sign", "logs:read"}, "")
	h.expect(h.do("POST", "/signatures", key, `{"id":"d1","type":"01"}`), 400, "organization-required")
	h.expect(h.do("POST", "/signatures", key, `{"id":"d1","type":"01"}`, "X-Organization-Id", "nope"), 404, "organization-not-found")
	signed := h.expect(h.do("POST", "/signatures", key, `{"id":"d1","type":"01"}`, "X-Organization-Id", org), 200).json(t)
	if signed["document_id"] != "d1" || signed["document_type"] != "01" || str(signed, "signature") == "" {
		t.Fatalf("signed = %v", signed)
	}
	// By tax id, with other separators.
	h.expect(h.do("POST", "/signatures", key, `{"id":"d2"}`, "X-Organization-Id", "0614 123456 101 2"), 200)
	h.expect(h.do("POST", "/signatures", key, `not json`, "X-Organization-Id", org), 422, "invalid-document")
	h.expect(h.do("POST", "/signatures", key, `<a/>`, "X-Organization-Id", org, "Content-Type", "text/plain"), 415, "unsupported-content-type")
	v := h.expect(h.do("POST", "/verifications", key, map[string]any{"signature": str(signed, "signature")}, "X-Organization-Id", org), 200).json(t)
	if v["valid"] != true || v["document_id"] != "d1" {
		t.Fatalf("verification = %v", v)
	}
	// No prod certificate.
	prodKey := h.key(ana, ws, "prod", []string{"sign"}, "")
	h.expect(h.do("POST", "/signatures", prodKey, `{"id":"p1"}`, "X-Organization-Id", org), 409, "certificate-not-active")
	// A key without sign.
	readKey := h.key(ana, ws, "test", []string{"organizations:read"}, "")
	h.expect(h.do("POST", "/signatures", readKey, `{"id":"x"}`, "X-Organization-Id", org), 403, "forbidden")
	// self is the key's space; a key never reaches another one.
	h.expect(h.do("GET", "/workspaces/self/organizations", readKey, nil), 200)
	h.expect(h.do("GET", "/workspaces/self/api-keys", readKey, nil), 403, "forbidden")

	// The log and the usage.
	log := h.expect(h.do("GET", "/workspaces/self/signatures", key, nil), 200).items(t)
	if len(log) != 2 || log[0]["document_id"] != "d2" || log[0]["organization_name"] != "Empresa" || log[0]["signed_by_workspace_id"] != ws {
		t.Fatalf("log = %v", log)
	}
	h.expect(h.do("GET", "/workspaces/self/signatures/"+str(log[1], "id"), key, nil), 200)
	usage := h.expect(h.do("GET", "/workspaces/"+ws+"/usage", ana, nil), 200).json(t)
	if items := usage["items"].([]any); len(items) != 1 || items[0].(map[string]any)["signatures"] != float64(2) {
		t.Fatalf("usage = %v", usage)
	}

	// Signing from the panel.
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/signatures?environment=test", ana, `{"id":"panel"}`), 200)
	log = h.expect(h.do("GET", "/workspaces/"+ws+"/signatures?limit=1", ana, nil), 200).items(t)
	if log[0]["user_id"] != "ana" || log[0]["api_key_id"] != nil {
		t.Fatalf("panel signature = %v", log[0])
	}

	// Revoking a key stops it at once.
	keys := h.expect(h.do("GET", "/workspaces/"+ws+"/api-keys", ana, nil), 200).items(t)
	if len(keys) != 3 || keys[0]["key"] != nil {
		t.Fatalf("keys = %v", keys)
	}
	var signKeyID string
	for _, k := range keys {
		if k["environment"] == "test" && len(k["scopes"].([]any)) == 2 {
			signKeyID = str(k, "id")
		}
	}
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/api-keys/"+signKeyID, ana, nil), 204)
	h.expect(h.do("POST", "/signatures", key, `{"id":"x"}`, "X-Organization-Id", org), 401, "unauthorized")

	// Deleting the organization; its log stays, without its name.
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/organizations/"+org, ana, nil), 204)
	log = h.expect(h.do("GET", "/workspaces/"+ws+"/signatures", ana, nil), 200).items(t)
	if len(log) != 3 || log[0]["organization_name"] != nil {
		t.Fatalf("log after delete = %v", log)
	}
}

func TestRolesAndOrganizationKeys(t *testing.T) {
	h := newHarness(t, nil)
	ana, beto := h.person("ana", auth.Claims{}), h.person("beto", auth.Claims{})
	ws := h.space(ana, "A")
	h.expect(h.do("GET", "/workspaces/"+ws, beto, nil), 404, "workspace-not-found")
	if err := h.db.AddMember(context.Background(), ws, "beto", model.Member); err != nil {
		t.Fatal(err)
	}
	org := h.organization(ana, ws, "E1", "1", "SV")
	other := h.organization(ana, ws, "E2", "2", "SV")
	h.certificate(ana, ws, org, "test", "1")
	h.certificate(ana, ws, org, "prod", "1")

	// A member reads, signs from the panel in test only, and changes nothing.
	h.expect(h.do("GET", "/workspaces/"+ws+"/organizations", beto, nil), 200)
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", beto, map[string]any{"name": "X", "tax_id": "3", "country": "SV"}), 403)
	h.expect(h.do("POST", "/workspaces/"+ws+"/api-keys", beto, map[string]any{"name": "k", "environment": "test", "scopes": []string{"sign"}}), 403)
	h.expect(h.do("PATCH", "/workspaces/"+ws, beto, map[string]any{"name": "B"}), 403)
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/signatures?environment=test", beto, `{"id":"t"}`), 200)
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/signatures?environment=prod", beto, `{"id":"p"}`), 403)
	members := h.expect(h.do("GET", "/workspaces/"+ws+"/members", ana, nil), 200).items(t)
	if len(members) != 2 {
		t.Fatalf("members = %v", members)
	}

	// A key of one organization reaches only it, and creates nothing wider.
	orgKey := h.key(ana, ws, "test", []string{"sign", "organizations:read", "keys:write", "keys:read"}, org)
	if list := h.expect(h.do("GET", "/workspaces/self/organizations", orgKey, nil), 200).items(t); len(list) != 1 || list[0]["id"] != org {
		t.Fatalf("an organization's key lists %v", list)
	}
	h.expect(h.do("GET", "/workspaces/self/organizations/"+other, orgKey, nil), 404, "organization-not-found")
	h.expect(h.do("POST", "/signatures", orgKey, `{"id":"x"}`, "X-Organization-Id", other), 404)
	h.expect(h.do("POST", "/signatures", orgKey, `{"id":"x"}`, "X-Organization-Id", org), 200)
	h.expect(h.do("POST", "/workspaces/self/api-keys", orgKey, map[string]any{"name": "k", "environment": "test", "scopes": []string{"sign"}}), 403)
	h.expect(h.do("POST", "/workspaces/self/api-keys", orgKey, map[string]any{"name": "k", "environment": "test", "scopes": []string{"logs:read"}, "organization_id": org}), 403)
	h.expect(h.do("POST", "/workspaces/self/api-keys", orgKey, map[string]any{"name": "k", "environment": "prod", "scopes": []string{"sign"}, "organization_id": org}), 403)
	h.expect(h.do("POST", "/workspaces/self/api-keys", orgKey, map[string]any{"name": "k", "environment": "test", "scopes": []string{"sign"}, "organization_id": org}), 201)
	h.expect(h.do("POST", "/workspaces/self/organizations", orgKey, map[string]any{"name": "X", "tax_id": "3", "country": "SV"}), 403)

	// Pagination.
	for i := range 5 {
		h.organization(ana, ws, fmt.Sprintf("P%d", i), fmt.Sprintf("p%d", i), "CR")
	}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		r := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations?limit=3&cursor="+cursor, ana, nil), 200).json(t)
		for _, it := range r["items"].([]any) {
			seen[str(it.(map[string]any), "id")] = true
		}
		next, _ := r["next_cursor"].(string)
		if next == "" {
			if pages != 2 {
				t.Fatalf("%d pages", pages+1)
			}
			break
		}
		cursor = next
	}
	if len(seen) != 7 {
		t.Fatalf("paginated %d organizations", len(seen))
	}
	h.expect(h.do("GET", "/workspaces/"+ws+"/organizations?limit=0", ana, nil), 400, "invalid-request")
	h.expect(h.do("GET", "/workspaces/"+ws+"/organizations?cursor=zzz", ana, nil), 400, "invalid-request")
}

func TestDelegations(t *testing.T) {
	h := newHarness(t, nil)
	ana, carl := h.person("ana", auth.Claims{}), h.person("carl", auth.Claims{})
	owner := h.space(ana, "Dueño")
	erp := h.space(carl, "ERP")
	org := h.organization(ana, owner, "Empresa", "0614", "SV")
	h.certificate(ana, owner, org, "test", "0614")

	erpKey := h.key(carl, erp, "test", []string{"sign", "logs:read", "certificates:read", "organizations:read"}, "")
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"x"}`, "X-Organization-Id", org), 404)

	h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": erp, "scopes": []string{"keys:write"}}), 400)
	h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": owner, "scopes": []string{"sign"}}), 400)
	d := h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": erp, "scopes": []string{"sign"}}), 201).json(t)
	if d["workspace_name"] != "ERP" || d["locked"] != false {
		t.Fatalf("delegation = %v", d)
	}
	h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": erp, "scopes": []string{"sign"}}), 409, "delegation-exists")

	// The delegate signs; the signature counts in its space.
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"by-erp"}`, "X-Organization-Id", "0614"), 200)
	usage := h.expect(h.do("GET", "/workspaces/self/usage", erpKey, nil), 200).json(t)
	if len(usage["items"].([]any)) != 1 {
		t.Fatalf("the delegate's usage = %v", usage)
	}
	if items := h.expect(h.do("GET", "/workspaces/"+owner+"/usage", ana, nil), 200).json(t)["items"].([]any); len(items) != 0 {
		t.Fatalf("the owner's usage counted the delegate's signature: %v", items)
	}
	// The owner sees it in its log, without the delegate's key.
	log := h.expect(h.do("GET", "/workspaces/"+owner+"/signatures", ana, nil), 200).items(t)
	if len(log) != 1 || log[0]["signed_by_workspace_id"] != erp || log[0]["api_key_id"] != nil {
		t.Fatalf("the owner's log = %v", log)
	}

	// What the delegation doesn't give.
	delegated := h.expect(h.do("GET", "/workspaces/"+erp+"/organizations?relation=delegated", carl, nil), 200).items(t)
	if len(delegated) != 1 || delegated[0]["relation"] != "delegated" || delegated[0]["delegation"] == nil {
		t.Fatalf("delegated = %v", delegated)
	}
	h.expect(h.do("GET", "/workspaces/"+erp+"/organizations?relation=owned", carl, nil), 200)
	h.expect(h.do("GET", "/workspaces/"+erp+"/organizations/"+org+"/certificates", carl, nil), 403)
	h.expect(h.do("PATCH", "/workspaces/"+erp+"/organizations/"+org, carl, map[string]any{"name": "Mía"}), 403, "not-organization-owner")
	h.expect(h.do("GET", "/workspaces/"+erp+"/organizations/"+org+"/delegations", carl, nil), 403, "not-organization-owner")

	// Not deleted while delegated; the owner removes an unlocked delegation.
	h.expect(h.do("DELETE", "/workspaces/"+owner+"/organizations/"+org, ana, nil), 409, "organization-has-delegations")
	h.expect(h.do("DELETE", "/workspaces/"+owner+"/organizations/"+org+"/delegations/"+str(d, "id"), ana, nil), 204)
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"x"}`, "X-Organization-Id", org), 404)

	// A locked one: the owner can't; the delegate renounces it.
	locked, err := h.db.CreateDelegation(context.Background(), org, erp, model.DelegationScopes, true)
	if err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("DELETE", "/workspaces/"+owner+"/organizations/"+org+"/delegations/"+locked.ID, ana, nil), 409, "delegation-locked")
	h.expect(h.do("GET", "/workspaces/"+erp+"/organizations/"+org+"/certificates", carl, nil), 200)
	h.expect(h.do("DELETE", "/workspaces/"+erp+"/organizations/"+org+"/delegations/"+locked.ID, carl, nil), 204)
	h.expect(h.do("GET", "/workspaces/"+erp+"/organizations/"+org, carl, nil), 404)
}

func TestCountries(t *testing.T) {
	h := newHarness(t, nil)
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "A")
	// ZZ is enabled but its signer isn't written.
	org := h.organization(ana, ws, "Z", "1", "ZZ")
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/certificates", ana,
		map[string]any{"environment": "test", "file": fakeCert("1")}), 501, "country-not-implemented")
	// An organization of a country this server doesn't offer (created
	// before SIGN_COUNTRIES changed) doesn't sign.
	if _, _, err := h.db.CreateOrganization(context.Background(), storeOrg(ws, "GT")); err != nil {
		t.Fatal(err)
	}
	key := h.key(ana, ws, "test", []string{"sign"}, "")
	h.expect(h.do("POST", "/signatures", key, `{"id":"x"}`, "X-Organization-Id", "GT-1"), 409, "country-not-enabled")
}

func TestHealthAndContract(t *testing.T) {
	h := newHarness(t, nil)
	h.expect(h.do("GET", "/health", "", nil), 200)
	r := h.expect(h.do("GET", "/openapi.yaml", "", nil), 200)
	if r.header.Get("Content-Type") != "application/yaml" || len(r.body) == 0 {
		t.Fatal("openapi.yaml")
	}
	h.expect(h.do("GET", "/nowhere", "", nil), 404, "not-found")
	// CORS for the panel only.
	req, _ := http.NewRequest("OPTIONS", h.http.URL+"/me", nil)
	req.Header.Set("Origin", "http://sign.sello.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://sign.sello.test" {
		t.Fatalf("preflight: %d %v", res.StatusCode, res.Header)
	}
}
