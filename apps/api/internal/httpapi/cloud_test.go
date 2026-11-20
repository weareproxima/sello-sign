package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/billing"
)

// fakeBilling is Sello's billing: it answers activations with the state
// it holds for each organization.
type fakeBilling struct {
	mu     sync.Mutex
	states map[string]string
}

func (f *fakeBilling) set(id string, version int, members string, maxMembers string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[id] = stateJSON(id, version, members, maxMembers)
}

func stateJSON(id string, version int, members, maxMembers string) string {
	return fmt.Sprintf(`{"workspace_id": %q, "version": %d, "closed": false, "name": "Empresa Oc", "country": "SV",
		"members": [%s], "plan": "PRO_STARTER",
		"limits": {"issuers": 5, "prod_signatures": 1000, "test_signatures": null, "members": %s, "requests_per_minute": 60, "retention_days": 30},
		"hard_caps": {"prod": 2000, "test": null}, "subscription": null}`, id, version, members, maxMembers)
}

func (f *fakeBilling) handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer billing-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/billing/sign/workspaces/")
	f.mu.Lock()
	body, ok := f.states[id]
	f.mu.Unlock()
	if r.Method != http.MethodPut || !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = io.WriteString(w, body)
}

func TestCloud(t *testing.T) {
	fb := &fakeBilling{states: map[string]string{}}
	billingSrv := httptest.NewServer(http.HandlerFunc(fb.handler))
	defer billingSrv.Close()

	var platform string
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
	platform = ps.ID
	h.server.PlatformWorkspace = platform

	// The platform registers two organizations of the same company, linked
	// to its organizations at the IdP, and a third of someone else.
	staff := h.person("staff", auth.Claims{})
	platformKey := h.key(staff, platform, "test", []string{"sign", "organizations:write", "organizations:read", "certificates:write"}, "")
	mk := func(name, tax, country, sello string) string {
		r := h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
			map[string]any{"name": name, "tax_id": tax, "country": country, "sello_workspace_id": sello}), 201)
		return str(r.json(t), "id")
	}
	oc := mk("Empresa Oc", "0614-1", "SV", "idp-oc")
	ocCR := mk("Empresa Oc Costa Rica", "3-101-1", "CR", "idp-oc-cr")
	lacteos := mk("Lácteos", "0614-2", "SV", "idp-lacteos")
	h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Dup", "tax_id": "9", "country": "SV", "sello_workspace_id": "idp-oc"}), 409, "sello-workspace-linked")
	h.expect(h.do("POST", "/workspaces/self/organizations/"+oc+"/certificates", platformKey,
		map[string]any{"environment": "test", "file": fakeCert("06141")}), 201)
	h.expect(h.do("POST", "/signatures", platformKey, `{"id":"before"}`, "X-Organization-Id", oc), 200)

	// The company's admin and a member, as the IdP's token says.
	orgs := []auth.SelloWorkspace{
		{ID: "idp-oc", Name: "Empresa Oc", Country: "SV", Role: "owner"},
		{ID: "idp-oc-cr", Name: "Empresa Oc Costa Rica", Country: "CR", Role: "admin"},
		{ID: "idp-new", Name: "Nueva", Country: "SV", Role: "member"},
	}
	admin := h.person("u-admin", auth.Claims{SelloSub: "s-admin", SelloWorkspaces: orgs})
	member := h.person("u-member", auth.Claims{SelloSub: "s-member", SelloWorkspaces: []auth.SelloWorkspace{{ID: "idp-oc", Role: "member"}}})
	late := h.person("u-late", auth.Claims{SelloSub: "s-late", SelloWorkspaces: []auth.SelloWorkspace{{ID: "idp-oc", Role: "member"}}})

	h.expect(h.do("POST", "/workspaces", admin, map[string]any{"name": "x"}), 400, "invalid-request")
	h.expect(h.do("POST", "/workspaces", admin, map[string]any{"sello_workspace_id": "idp-other"}), 403, "forbidden")
	// A member doesn't open a space that doesn't exist yet.
	h.expect(h.do("POST", "/workspaces", member, map[string]any{"sello_workspace_id": "idp-oc"}), 403, "ask-admin")
	// Billing doesn't know it yet.
	h.expect(h.do("POST", "/workspaces", admin, map[string]any{"sello_workspace_id": "idp-oc"}), 409, "sello-organization-unknown")

	fb.set("idp-oc", 3, `{"account": "s-admin", "role": "OWNER"}, {"account": "s-member", "role": "MEMBER"}, {"account": "s-late", "role": "MEMBER"}`, "2")
	opened := h.expect(h.do("POST", "/workspaces", admin, map[string]any{"sello_workspace_id": "idp-oc"}), 201).json(t)
	ws := str(opened, "id")
	if opened["plan"] != "pro_starter" || opened["role"] != "owner" || opened["default_country"] != "SV" {
		t.Fatalf("opened = %v", opened)
	}
	h.expect(h.do("POST", "/workspaces", admin, map[string]any{"sello_workspace_id": "idp-oc"}), 200)
	h.expect(h.do("POST", "/workspaces", member, map[string]any{"sello_workspace_id": "idp-oc"}), 200)
	// The plan admits two members: the third seat isn't in.
	h.expect(h.do("POST", "/workspaces", late, map[string]any{"sello_workspace_id": "idp-oc"}), 409, "workspace-full")

	options := h.expect(h.do("GET", "/me/sello-organizations", admin, nil), 200).items(t)
	statuses := map[string]string{}
	for _, o := range options {
		statuses[str(o, "id")] = str(o, "status")
	}
	if statuses["idp-oc"] != "open" || statuses["idp-oc-cr"] != "available" || statuses["idp-new"] != "ask" {
		t.Fatalf("statuses = %v", statuses)
	}
	if lateOpts := h.expect(h.do("GET", "/me/sello-organizations", late, nil), 200).items(t); str(lateOpts[0], "status") != "full" {
		t.Fatalf("late = %v", lateOpts)
	}

	// Billing pushes: stale, applied (a third seat), unknown, unauthorized.
	push := func(id string, version int, maxMembers, token string) response {
		return h.do("PUT", "/api/billing/organizations/"+id, token,
			stateJSON(id, version, `{"account": "s-admin", "role": "OWNER"}, {"account": "s-member", "role": "MEMBER"}, {"account": "s-late", "role": "MEMBER"}`, maxMembers))
	}
	h.expect(push("idp-oc", 3, "3", "push-token"), 204)
	h.expect(h.do("GET", "/workspaces/"+ws, late, nil), 404) // stale: still two seats
	h.expect(push("idp-oc", 4, "3", "push-token"), 204)
	h.expect(h.do("GET", "/workspaces/"+ws, late, nil), 200)
	h.expect(push("idp-unknown", 1, "3", "push-token"), 404, UnknownOrganizationType)
	h.expect(push("idp-oc", 5, "3", "billing-token"), 401)
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token", stateJSON("idp-other", 9, "", "1")), 400)

	// «Usar en Sello Sign»: a member can't; the admin sees what they manage.
	h.expect(h.do("GET", "/workspaces/"+ws+"/claimable-organizations", member, nil), 403)
	claimable := h.expect(h.do("GET", "/workspaces/"+ws+"/claimable-organizations", admin, nil), 200).items(t)
	if len(claimable) != 2 {
		t.Fatalf("claimable = %v", claimable)
	}
	h.expect(h.do("POST", "/workspaces/"+ws+"/organization-claims", admin, map[string]any{"organization_ids": []string{oc, lacteos}}), 404, "organization-not-claimable")
	claimed := h.expect(h.do("POST", "/workspaces/"+ws+"/organization-claims", admin, map[string]any{"organization_ids": []string{oc, ocCR}}), 200).items(t)
	if len(claimed) != 2 || claimed[0]["relation"] != "owned" || claimed[0]["workspace_id"] != ws {
		t.Fatalf("claimed = %v", claimed)
	}
	// With its certificate and its history.
	o := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+oc, admin, nil), 200).json(t)
	if o["certificates"].(map[string]any)["test"] == nil {
		t.Fatalf("the certificate didn't move: %v", o)
	}
	if log := h.expect(h.do("GET", "/workspaces/"+ws+"/signatures", admin, nil), 200).items(t); len(log) != 1 || log[0]["signed_by_workspace_id"] != platform {
		t.Fatalf("history = %v", log)
	}
	// The platform keeps signing through a locked delegation.
	h.expect(h.do("POST", "/signatures", platformKey, `{"id":"after"}`, "X-Organization-Id", "0614-1"), 200)
	ds := h.expect(h.do("GET", "/workspaces/"+ws+"/organizations/"+oc+"/delegations", admin, nil), 200).items(t)
	if len(ds) != 1 || ds[0]["workspace_id"] != platform || ds[0]["locked"] != true {
		t.Fatalf("delegations = %v", ds)
	}
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/organizations/"+oc+"/delegations/"+str(ds[0], "id"), admin, nil), 409, "delegation-locked")
	// Already claimed: not again.
	h.expect(h.do("POST", "/workspaces/"+ws+"/organization-claims", admin, map[string]any{"organization_ids": []string{oc}}), 404, "organization-not-claimable")
	if left := h.expect(h.do("GET", "/workspaces/"+ws+"/claimable-organizations", admin, nil), 200).items(t); len(left) != 0 {
		t.Fatalf("left = %v", left)
	}
	// The usage of each stays in the space of the key that signed.
	if items := h.expect(h.do("GET", "/workspaces/"+ws+"/usage", admin, nil), 200).json(t)["items"].([]any); len(items) != 0 {
		t.Fatalf("the company's usage counted the platform's: %v", items)
	}
	// What billing is told each hour.
	report, err := h.db.UsageReport(ctx, []string{currentMonth()})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range report {
		if e.WorkspaceID == "idp-oc" {
			t.Fatalf("the company was reported the platform's usage: %v", report)
		}
	}

	// A closed organization keeps its space and stops signing; what the
	// platform delegates goes back to it (TestCloudPlatformLinks).
	own := h.organization(admin, ws, "Propia", "0614-9", "SV")
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+own+"/certificates", admin,
		map[string]any{"environment": "test", "file": fakeCert("06149")}), 201)
	companyKey := h.key(admin, ws, "test", []string{"sign"}, "")
	h.expect(h.do("POST", "/signatures", companyKey, `{"id":"own"}`, "X-Organization-Id", own), 200)
	closed := strings.Replace(stateJSON("idp-oc", 10, "", "3"), `"closed": false`, `"closed": true`, 1)
	h.expect(h.do("PUT", "/api/billing/organizations/idp-oc", "push-token", closed), 204)
	h.expect(h.do("POST", "/signatures", companyKey, `{"id":"own"}`, "X-Organization-Id", own), 403, "workspace-closed")
	h.expect(h.do("POST", "/signatures", companyKey, `{"id":"own"}`, "X-Organization-Id", oc), 404, "organization-not-found")
	if got := h.expect(h.do("GET", "/workspaces/"+ws, admin, nil), 200).json(t); got["closed"] != true {
		t.Fatalf("closed = %v", got)
	}
}

func currentMonth() string { return monthNow() }
