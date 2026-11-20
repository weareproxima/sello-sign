package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/billing"
)

// TestCloudPlatformLinks covers what the model decided for the platform's
// space beyond «Usar en Sello Sign»: the platform registering an
// organization a company already has in Sign (the reverse order), and a
// closing space giving back what the platform delegates.
func TestCloudPlatformLinks(t *testing.T) {
	fb := &fakeBilling{states: map[string]string{}}
	billingSrv := httptest.NewServer(http.HandlerFunc(fb.handler))
	defer billingSrv.Close()

	h := newHarness(t, func(d *Deps) {
		d.Cloud = true
		d.Billing = billing.NewClient(billingSrv.URL, "billing-token")
		d.BillingPushToken = "push-token"
	})
	ctx := context.Background()
	ps, err := h.db.CreateWorkspace(ctx, "Sello", nil, "staff")
	if err != nil {
		t.Fatal(err)
	}
	platform := ps.ID
	h.server.PlatformWorkspace = platform
	staff := h.person("staff", auth.Claims{})
	platformKey := h.key(staff, platform, "test", []string{"sign", "organizations:write", "organizations:read", "certificates:write"}, "")

	// The company contracts Sign first and links its organization of the
	// IdP to one of its own.
	admin := h.person("u-admin", auth.Claims{SelloSub: "s-admin", SelloWorkspaces: []auth.SelloWorkspace{
		{ID: "idp-oc", Name: "Empresa Oc", Country: "SV", Role: "owner"},
	}})
	fb.set("idp-oc", 1, `{"account": "s-admin", "role": "OWNER"}`, "3")
	ws := str(h.expect(h.do("POST", "/workspaces", admin, map[string]any{"sello_workspace_id": "idp-oc"}), 201).json(t), "id")
	org := str(h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", admin,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-1", "country": "SV", "sello_workspace_id": "idp-oc"}), 201).json(t), "id")
	h.certificate(admin, ws, org, "test", "06141")
	unlinked := h.organization(admin, ws, "Sin Sello", "0614-2", "SV")

	// It delegates it to an accountant's space, and has keys of it.
	accountant, err := h.db.CreateWorkspace(ctx, "Contador", nil, "u-accountant")
	if err != nil {
		t.Fatal(err)
	}
	accountantToken := h.person("u-accountant", auth.Claims{})
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/delegations", admin,
		map[string]any{"workspace_id": accountant.ID, "scopes": []string{"sign"}}), 201)
	accountantKey := h.key(accountantToken, accountant.ID, "test", []string{"sign"}, "")
	onlyKey := h.key(admin, ws, "test", []string{"sign"}, org)
	companyKey := h.key(admin, ws, "test", []string{"sign", "organizations:read"}, "")
	h.expect(h.do("POST", "/signatures", onlyKey, `{"id":"company"}`, "X-Organization-Id", org), 200)

	// Any other space that names it is still a conflict, even for a person
	// who manages it at the IdP.
	other, err := h.db.CreateWorkspace(ctx, "Otro", nil, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/workspaces/"+other.ID+"/organizations", admin,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-1", "country": "SV", "sello_workspace_id": "idp-oc"}), 409, "sello-workspace-linked")

	// Another tax id or country for that link is a conflict, not a delegation.
	h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-2", "country": "SV", "sello_workspace_id": "idp-oc"}), 409, "organization-mismatch")
	h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-1", "country": "CR", "sello_workspace_id": "idp-oc"}), 409, "organization-mismatch")

	// The platform registers it later: the same organization, delegated,
	// once however often it asks.
	for range 2 {
		got := h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
			map[string]any{"name": "Empresa Oc", "tax_id": "06141", "country": "SV", "sello_workspace_id": "idp-oc"}), 200).json(t)
		if got["id"] != org || got["relation"] != "delegated" || got["workspace_id"] != ws {
			t.Fatalf("registered = %v", got)
		}
	}
	ds := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+org+"/delegations", admin, nil), 200).items(t)
	locked := 0
	for _, d := range ds {
		if d["workspace_id"] == platform {
			locked++
			if d["locked"] != true {
				t.Fatalf("the platform's delegation isn't locked: %v", d)
			}
			h.expect(h.do("DELETE", "/workspaces/"+ws+"/organizations/"+org+"/delegations/"+str(d, "id"), admin, nil), 409, "delegation-locked")
		}
	}
	if len(ds) != 2 || locked != 1 {
		t.Fatalf("delegations = %v", ds)
	}
	if orgs := h.expect(h.do("GET", "/workspaces/self/organizations", platformKey, nil), 200).items(t); len(orgs) != 1 {
		t.Fatalf("the platform has a second organization: %v", orgs)
	}
	h.expect(h.do("POST", "/signatures", platformKey, `{"id":"platform"}`, "X-Organization-Id", "0614-1"), 200)

	// Billing closes the company's space: what the platform delegates goes
	// back to it, with its certificate and its history; the rest stays.
	closed := func(version int) string {
		return strings.Replace(stateJSON("idp-oc", version, "", "3"), `"closed": false`, `"closed": true`, 1)
	}
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token", closed(2)), 204)
	check := func() {
		t.Helper()
		got := h.expect(h.do("GET", "/workspaces/self/organizations/"+org, platformKey, nil), 200).json(t)
		if got["relation"] != "owned" || got["workspace_id"] != platform || got["certificates"].(map[string]any)["test"] == nil {
			t.Fatalf("returned = %v", got)
		}
		if ds := h.expect(h.do("GET", "/workspaces/"+platform+"/organizations/"+org+"/delegations", staff, nil), 200).items(t); len(ds) != 0 {
			t.Fatalf("delegations left = %v", ds)
		}
		log := h.expect(h.do("GET", "/workspaces/"+platform+"/signatures", staff, nil), 200).items(t)
		signers := map[any]int{}
		for _, e := range log {
			signers[e["signed_by_workspace_id"]]++
		}
		if signers[ws] != 1 || signers[platform] != 1 {
			t.Fatalf("history = %v", log)
		}
		h.expect(h.do("POST", "/signatures", onlyKey, `{"id":"x"}`, "X-Organization-Id", org), 401, "unauthorized")
		h.expect(h.do("POST", "/signatures", companyKey, `{"id":"x"}`, "X-Organization-Id", org), 404, "organization-not-found")
		h.expect(h.do("POST", "/signatures", accountantKey, `{"id":"x"}`, "X-Organization-Id", org), 404, "organization-not-found")
		left := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations", admin, nil), 200).items(t)
		if len(left) != 1 || left[0]["id"] != unlinked {
			t.Fatalf("left in the closed space = %v", left)
		}
		h.expect(h.do("POST", "/signatures", platformKey, `{"id":"again"}`, "X-Organization-Id", org), 200)
	}
	check()
	// The same state again, and a newer one still closed: nothing changes.
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token", closed(2)), 204)
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token", closed(3)), 204)
	// Reopening doesn't bring it back: the company claims it again.
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token",
		stateJSON("idp-oc", 4, `{"account": "s-admin", "role": "OWNER"}`, "3")), 204)
	h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+org, admin, nil), 404)
	claimable := h.expect(h.do("GET", "/workspaces/"+ws+"/claimable-organizations", admin, nil), 200).items(t)
	if len(claimable) != 1 || claimable[0]["id"] != org {
		t.Fatalf("claimable = %v", claimable)
	}
}
