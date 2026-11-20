package httpapi

import (
	"context"
	"testing"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/auth"
)

// rowsOf counts what's left of a space, and of its organization, in every
// table that names them.
func rowsOf(t *testing.T, h *harness, ws, org string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for name, where := range map[string]string{
		"workspaces":        `id = $1 OR $2::uuid IS NULL`,
		"workspace_members": `workspace_id = $1 OR $2::uuid IS NULL`,
		"organizations":     `workspace_id = $1 OR id = $2`,
		"certificates":      `organization_id = $2 OR $1::uuid IS NULL`,
		"delegations":       `workspace_id = $1 OR organization_id = $2`,
		"api_keys":          `workspace_id = $1 OR organization_id = $2`,
		"invitations":       `workspace_id = $1 OR $2::uuid IS NULL`,
		"signing_log":       `signed_by_workspace_id = $1 OR $2::uuid IS NULL`,
		"usage_counters":    `workspace_id = $1 OR $2::uuid IS NULL`,
		"workspace_usage":   `workspace_id = $1 OR $2::uuid IS NULL`,
		"audit_events":      `workspace_id = $1 OR $2::uuid IS NULL`,
	} {
		var n int
		if err := h.db.Pool().QueryRow(context.Background(), `SELECT count(*) FROM `+name+` WHERE `+where, ws, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[name] = n
	}
	return out
}

func TestDeleteWorkspace(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	carla := h.person("carla", auth.Claims{})

	// Ana's space, with Beto (admin) and Carla (member), an organization
	// with its certificate, keys, an invitation and what it signed.
	ws := h.space(ana, "Proveedor A")
	for user, role := range map[string]string{"beto": "admin", "carla": "member"} {
		if err := h.db.AddMember(ctx, ws, user, "member"); err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.Pool().Exec(ctx, `UPDATE workspace_members SET role = $3 WHERE workspace_id = $1 AND user_id = $2`, ws, user, role); err != nil {
			t.Fatal(err)
		}
	}
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	h.certificate(ana, ws, org, "test", "06141234561012")
	key := h.key(ana, ws, "test", []string{"sign", "organizations:read"}, "")
	h.expect(h.do("POST", "/signatures", key, `{"id":"d1"}`, "X-Organization-Id", org), 200)
	h.expect(h.do("POST", "/workspaces/"+ws+"/invitations", ana, map[string]any{"email": "dora@example.com", "role": "member"}), 201)

	// Beto's space delegates one of its organizations to Ana's, and uses
	// Ana's by delegation, with a key of only that one.
	other := h.space(beto, "Otro")
	theirs := h.organization(beto, other, "Ajena", "1", "CR")
	h.expect(h.do("POST", "/workspaces/"+other+"/organizations/"+theirs+"/delegations", beto,
		map[string]any{"workspace_id": ws, "scopes": []string{"sign"}}), 201)
	out := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": other, "scopes": []string{"sign"}}), 201).json(t)
	otherKey := h.key(beto, other, "test", []string{"sign"}, org)
	h.expect(h.do("POST", "/signatures", otherKey, `{"id":"d2"}`, "X-Organization-Id", org), 200)

	del := func(as string, body any) response {
		return h.do("DELETE", "/workspaces/"+ws, as, body)
	}
	confirm := map[string]any{"confirmation": "Proveedor A"}

	// Only an owner, with the token, and with the space's name.
	h.expect(h.do("DELETE", "/workspaces/self", key, confirm), 403, "forbidden")
	h.expect(del(beto, confirm), 403, "forbidden")
	h.expect(del(carla, confirm), 403, "forbidden")
	h.expect(del(h.person("eva", auth.Claims{}), confirm), 404, "workspace-not-found")
	h.expect(del(ana, nil), 400, "invalid-request")
	h.expect(del(ana, map[string]any{}), 400, "confirmation-mismatch")
	h.expect(del(ana, map[string]any{"confirmation": "proveedor a"}), 400, "confirmation-mismatch")
	h.expect(del(ana, map[string]any{"confirmation": "Otro"}), 400, "confirmation-mismatch")

	// While another space uses its organization, nothing changes.
	h.expect(del(ana, confirm), 409, "workspace-has-delegations")
	h.expect(h.do("POST", "/signatures", key, `{"id":"d3"}`, "X-Organization-Id", org), 200)
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/organizations/"+org+"/delegations/"+str(out, "id"), ana, nil), 204)

	// Deleted, blanks around the name aside.
	h.expect(del(ana, map[string]any{"confirmation": "  Proveedor A "}), 204)
	h.expect(h.do("GET", "/workspaces/"+ws, ana, nil), 404, "workspace-not-found")
	h.expect(del(ana, confirm), 404, "workspace-not-found")
	// Its keys stop at once, also another space's key of its organization.
	h.expect(h.do("POST", "/signatures", key, `{"id":"d4"}`, "X-Organization-Id", org), 401, "unauthorized")
	h.expect(h.do("GET", "/workspaces/self", otherKey, nil), 401, "unauthorized")
	for table, n := range rowsOf(t, h, ws, org) {
		if n != 0 {
			t.Errorf("%d rows of the deleted space in %s", n, table)
		}
	}
	me := h.expect(h.do("GET", "/me", beto, nil), 200).json(t)
	if spaces := me["workspaces"].([]any); len(spaces) != 1 {
		t.Fatalf("Beto's spaces = %v", spaces)
	}

	// The other space keeps its organization, and what it signed with Ana's
	// stays in its log and its usage, without the organization's name.
	h.expect(h.do("GET", "/workspaces/"+other+"/organizations/"+theirs, beto, nil), 200)
	log := h.expect(h.do("GET", "/workspaces/"+other+"/signatures", beto, nil), 200).items(t)
	if len(log) != 1 || log[0]["organization_id"] != org || log[0]["organization_name"] != nil {
		t.Fatalf("the other space's log = %v", log)
	}
	usage := h.expect(h.do("GET", "/workspaces/"+other+"/usage", beto, nil), 200).json(t)
	if items := usage["items"].([]any); len(items) != 1 {
		t.Fatalf("the other space's usage = %v", usage)
	}
}

func TestDeleteWorkspaceProtected(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	staff := h.person("staff", auth.Claims{})
	ps, err := h.db.CreateWorkspace(ctx, "Sello", nil, "staff")
	if err != nil {
		t.Fatal(err)
	}
	h.server.PlatformWorkspace = ps.ID
	h.expect(h.do("DELETE", "/workspaces/"+ps.ID, staff, map[string]any{"confirmation": "Sello"}), 409, "platform-workspace")

	// The space of an organization of Sello: its plan and members are
	// Sello's.
	ana := h.person("ana", auth.Claims{})
	linked := h.space(ana, "De Sello")
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET sello_workspace_id = 'idp-x' WHERE id = $1`, linked); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("DELETE", "/workspaces/"+linked, ana, map[string]any{"confirmation": "De Sello"}), 409, "workspace-managed-by-sello")
	h.expect(h.do("GET", "/workspaces/"+linked, ana, nil), 200)
}

// In the cloud, the organizations the platform holds a locked delegation
// of go back to it before the space goes, as when it closes.
func TestDeleteWorkspaceReturnsToPlatform(t *testing.T) {
	h := newHarness(t, func(d *Deps) { d.Cloud = true })
	ctx := context.Background()
	ps, err := h.db.CreateWorkspace(ctx, "Sello", nil, "staff")
	if err != nil {
		t.Fatal(err)
	}
	platform := ps.ID
	h.server.PlatformWorkspace = platform
	staff := h.person("staff", auth.Claims{})
	platformKey := h.key(staff, platform, "test", []string{"sign", "organizations:write", "organizations:read", "certificates:write"}, "")
	oc := str(h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-1", "country": "SV", "sello_workspace_id": "idp-oc"}), 201).json(t), "id")
	h.certificate(platformKey, "self", oc, "test", "06141")

	// A space without an organization of the IdP (one the console made)
	// brings it with «Usar en Sello Sign».
	admin := h.person("u-admin", auth.Claims{SelloSub: "s-admin", SelloWorkspaces: []auth.SelloWorkspace{
		{ID: "idp-oc", Name: "Empresa Oc", Country: "SV", Role: "owner"},
	}})
	ws, err := h.db.CreateWorkspace(ctx, "Propio", nil, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/workspaces/"+ws.ID+"/organization-claims", admin, map[string]any{"organization_ids": []string{oc}}), 200)
	own := h.organization(admin, ws.ID, "Propia", "0614-9", "SV")
	onlyKey := h.key(admin, ws.ID, "test", []string{"sign"}, oc)
	h.expect(h.do("POST", "/signatures", onlyKey, `{"id":"company"}`, "X-Organization-Id", oc), 200)

	h.expect(h.do("DELETE", "/workspaces/"+ws.ID, admin, map[string]any{"confirmation": "Propio"}), 204)
	// The platform owns it again, with its certificate, and signs with it;
	// the space's own organization went with it.
	back := h.expect(h.do("GET", "/workspaces/self/organizations/"+oc, platformKey, nil), 200).json(t)
	if back["workspace_id"] != platform || back["relation"] != "owned" {
		t.Fatalf("returned = %v", back)
	}
	h.expect(h.do("POST", "/signatures", platformKey, `{"id":"platform"}`, "X-Organization-Id", oc), 200)
	h.expect(h.do("POST", "/signatures", onlyKey, `{"id":"again"}`, "X-Organization-Id", oc), 401, "unauthorized")
	// The platform's audit log says the deleted space gave it back, never
	// who deleted it.
	returned := one(t, h.events(staff, platform, ""), audit.OrganizationReturned)
	if returned.Actor.Type != "workspace" || returned.actorID() != ws.ID || *returned.Actor.Name != "Propio" ||
		returned.Actor.Email != nil || returned.targetID() != oc {
		t.Fatalf("returned = %+v", returned)
	}
	var n int
	if err := h.db.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM organizations WHERE id = $1)
		+ (SELECT count(*) FROM delegations WHERE organization_id = $2)`, own, oc).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d rows remain", n)
	}
}
