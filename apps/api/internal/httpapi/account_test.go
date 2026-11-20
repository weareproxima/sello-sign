package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/auth"
)

func TestRemoveMember(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	carla := h.person("carla", auth.Claims{})
	ws := h.space(ana, "Proveedor A")
	for user, role := range map[string]string{"beto": "admin", "carla": "member", "dora": "owner"} {
		if err := h.db.AddMember(ctx, ws, user, "member"); err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.Pool().Exec(ctx, `UPDATE workspace_members SET role = $3 WHERE workspace_id = $1 AND user_id = $2`, ws, user, role); err != nil {
			t.Fatal(err)
		}
	}
	m := func(user string) string { return "/workspaces/" + ws + "/members/" + user }

	// A key never removes anyone.
	key := h.key(ana, ws, "test", []string{"keys:read", "keys:write"}, "")
	h.expect(h.do("DELETE", m("carla"), key, nil), 403, "forbidden")
	// A member removes nobody but leaves by themselves.
	h.expect(h.do("DELETE", m("beto"), carla, nil), 403, "forbidden")
	h.expect(h.do("DELETE", m("nadie"), ana, nil), 404, "member-not-found")
	// An admin removes members and admins, never an owner.
	h.expect(h.do("DELETE", m("dora"), beto, nil), 403, "forbidden")
	h.expect(h.do("DELETE", m("carla"), beto, nil), 204)
	h.expect(h.do("GET", "/workspaces/"+ws, carla, nil), 404, "workspace-not-found")
	h.expect(h.do("DELETE", m("carla"), beto, nil), 404, "member-not-found")
	// An owner removes an owner, but never the last one; the last owner
	// can't leave either.
	h.expect(h.do("DELETE", m("dora"), ana, nil), 204)
	h.expect(h.do("DELETE", m("ana"), ana, nil), 409, "last-owner")
	h.expect(h.do("DELETE", m("beto"), beto, nil), 204)
	members := h.expect(h.do("GET", "/workspaces/"+ws+"/members", ana, nil), 200).items(t)
	if len(members) != 1 || members[0]["user_id"] != "ana" {
		t.Fatalf("members = %v", members)
	}

	// A space of an organization of Sello: its members are billing's.
	linked := h.space(ana, "De Sello")
	if err := h.db.AddMember(ctx, linked, "beto", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET sello_workspace_id = 'idp-x' WHERE id = $1`, linked); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("DELETE", "/workspaces/"+linked+"/members/beto", ana, nil), 409, "members-managed-by-sello")
	h.expect(h.do("DELETE", "/workspaces/"+linked+"/members/beto", beto, nil), 409, "members-managed-by-sello")
}

func TestRenewInvitation(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	carla := h.person("carla", auth.Claims{})
	ws := h.space(ana, "Proveedor A")
	if err := h.db.AddMember(ctx, ws, "beto", "admin"); err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + ws + "/invitations"
	inv := h.expect(h.do("POST", path, ana, map[string]any{"email": "carla@example.com", "role": "member"}), 201).json(t)
	if _, err := h.db.Pool().Exec(ctx, `UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE id = $1`, str(inv, "id")); err != nil {
		t.Fatal(err)
	}

	// Renewed: the same invitation, pending again, with another token; the
	// old link stops working.
	renewed := h.expect(h.do("POST", path+"/"+str(inv, "id")+"/renewals", beto, nil), 200).json(t)
	if renewed["id"] != inv["id"] || renewed["status"] != "pending" || !auth.IsInvitationToken(str(renewed, "token")) ||
		renewed["token"] == inv["token"] || str(renewed, "expires_at") <= str(inv, "expires_at") {
		t.Fatalf("renewed = %v (was %v)", renewed, inv)
	}
	h.expect(h.do("POST", "/invitation-lookups", carla, map[string]any{"token": str(inv, "token")}), 404, "invitation-not-found")
	if list := h.expect(h.do("GET", path, ana, nil), 200).items(t); len(list) != 1 || list[0]["status"] != "pending" {
		t.Fatalf("list = %v", list)
	}
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": str(renewed, "token")}), 200)
	// Accepted, it can't be renewed; nor one that doesn't exist.
	h.expect(h.do("POST", path+"/"+str(inv, "id")+"/renewals", ana, nil), 404, "invitation-not-found")
	h.expect(h.do("POST", path+"/nope/renewals", ana, nil), 404, "invitation-not-found")

	// Only an owner renews an owner's; a member renews nothing.
	owner := h.expect(h.do("POST", path, ana, map[string]any{"email": "dora@example.com", "role": "owner"}), 201).json(t)
	h.expect(h.do("POST", path+"/"+str(owner, "id")+"/renewals", beto, nil), 403, "forbidden")
	h.expect(h.do("POST", path+"/"+str(owner, "id")+"/renewals", carla, nil), 403, "forbidden")
	h.expect(h.do("POST", path+"/"+str(owner, "id")+"/renewals", ana, nil), 200)
	// A full space: the renewed one couldn't be accepted.
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET max_members = 3 WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", path+"/"+str(owner, "id")+"/renewals", ana, nil), 409, "workspace-full")
}

func TestVerifyWithToken(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	carla := h.person("carla", auth.Claims{})
	ws := h.space(ana, "Dueño")
	if err := h.db.AddMember(ctx, ws, "carla", "member"); err != nil {
		t.Fatal(err)
	}
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	h.certificate(ana, ws, org, "test", "06141234561012")
	signed := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/signatures?environment=test", ana, `{"id":"d1","type":"01"}`), 200).json(t)
	sig := str(signed, "signature")
	verify := func(as, ref string, body map[string]any) response {
		return h.do("POST", "/verifications", as, body, "X-Organization-Id", ref)
	}

	// Any member of the owner, by id or tax id; the environment is the body's.
	v := h.expect(verify(carla, org, map[string]any{"signature": sig, "environment": "test"}), 200).json(t)
	if v["valid"] != true || v["document_id"] != "d1" {
		t.Fatalf("verification = %v", v)
	}
	h.expect(verify(ana, "0614 123456 101 2", map[string]any{"signature": sig, "environment": "test"}), 200)
	h.expect(verify(ana, org, map[string]any{"signature": sig}), 400, "invalid-request")
	h.expect(verify(ana, org, map[string]any{"signature": sig, "environment": "dev"}), 400, "invalid-request")
	h.expect(verify(ana, org, map[string]any{"signature": sig, "environment": "prod"}), 409, "certificate-not-active")
	h.expect(verify(ana, "", map[string]any{"signature": sig, "environment": "test"}), 400, "organization-required")
	// Altered, it's an answer, not an error.
	tampered := strings.Replace(sig, "d1", "d2", 1)
	if v := h.expect(verify(ana, org, map[string]any{"signature": tampered, "environment": "test"}), 200).json(t); v["valid"] == true && tampered != sig {
		t.Fatalf("a tampered signature verifies: %v", v)
	}
	// Nobody else's organization.
	h.expect(verify(beto, org, map[string]any{"signature": sig, "environment": "test"}), 404, "organization-not-found")

	// A delegate's members, if the delegation signs or reads certificates.
	other := h.space(beto, "Delegado")
	d := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": other, "scopes": []string{"certificates:write"}}), 201).json(t)
	h.expect(verify(beto, org, map[string]any{"signature": sig, "environment": "test"}), 403, "forbidden")
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/organizations/"+org+"/delegations/"+str(d, "id"), ana, nil), 204)
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": other, "scopes": []string{"certificates:read"}}), 201)
	h.expect(verify(beto, org, map[string]any{"signature": sig, "environment": "test"}), 200)

	// An expired certificate still verifies what it signed.
	if _, err := h.db.Pool().Exec(ctx, `UPDATE certificates SET not_after = now() - interval '1 day' WHERE organization_id = $1`, org); err != nil {
		t.Fatal(err)
	}
	h.expect(verify(ana, org, map[string]any{"signature": sig, "environment": "test"}), 200)
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/signatures?environment=test", ana, `{"id":"d2"}`), 409, "certificate-expired")

	// A key keeps its environment.
	key := h.key(ana, ws, "test", []string{"sign"}, "")
	h.expect(verify(key, org, map[string]any{"signature": sig}), 200)
	h.expect(verify(key, org, map[string]any{"signature": sig, "environment": "test"}), 200)
	h.expect(verify(key, org, map[string]any{"signature": sig, "environment": "prod"}), 400, "invalid-request")

	// Nothing of it is logged.
	log := h.expect(h.do("GET", "/workspaces/"+ws+"/signatures", ana, nil), 200).items(t)
	if len(log) != 1 {
		t.Fatalf("verifying logged something: %v", log)
	}
}

func TestExportWorkspace(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{Name: "Ana"})
	beto := h.person("beto", auth.Claims{})
	ws := h.space(ana, "Proveedor A")
	if err := h.db.AddMember(ctx, ws, "beto", "member"); err != nil {
		t.Fatal(err)
	}
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	h.certificate(ana, ws, org, "test", "06141234561012")
	other := h.space(beto, "Otro")
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": other, "scopes": []string{"sign"}}), 201)
	theirs := h.organization(beto, other, "Ajena", "1", "CR")
	h.certificate(beto, other, theirs, "test", "1")
	h.expect(h.do("POST", "/workspaces/"+other+"/organizations/"+theirs+"/delegations", beto,
		map[string]any{"workspace_id": ws, "scopes": []string{"sign"}}), 201)
	key := h.key(ana, ws, "test", []string{"sign"}, "")
	revoked := h.key(ana, ws, "test", []string{"sign"}, "")
	keys := h.expect(h.do("GET", "/workspaces/"+ws+"/api-keys", ana, nil), 200).items(t)
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/api-keys/"+str(keys[0], "id"), ana, nil), 204)
	inv := h.expect(h.do("POST", "/workspaces/"+ws+"/invitations", ana, map[string]any{"email": "carla@example.com", "role": "member"}), 201).json(t)
	// More than one batch of the log.
	for i := range exportTestSignatures {
		h.expect(h.do("POST", "/signatures", key, `{"id":"d`+string(rune('a'+i%26))+`"}`, "X-Organization-Id", org), 200)
	}

	h.expect(h.do("GET", "/workspaces/"+ws+"/export", beto, nil), 403, "forbidden")
	h.expect(h.do("GET", "/workspaces/self/export", key, nil), 403, "forbidden")
	r := h.expect(h.do("GET", "/workspaces/"+ws+"/export", ana, nil), 200)
	if cd := r.header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="sello-sign-`+ws) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	for _, secret := range []string{"hash", "material", "password", key, revoked, str(inv, "token")} {
		if strings.Contains(string(r.body), secret) {
			t.Fatalf("the export carries %q: %s", secret, r.body)
		}
	}
	var e struct {
		Format        string           `json:"format"`
		Workspace     map[string]any   `json:"workspace"`
		Organizations []map[string]any `json:"organizations"`
		APIKeys       []map[string]any `json:"api_keys"`
		Members       []map[string]any `json:"members"`
		Invitations   []map[string]any `json:"invitations"`
		Usage         []map[string]any `json:"usage"`
		Signatures    []map[string]any `json:"signatures"`
	}
	if err := json.Unmarshal(r.body, &e); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if e.Format != exportFormat || e.Workspace["id"] != ws || len(e.Members) != 2 || len(e.Invitations) != 1 ||
		len(e.APIKeys) != 2 || len(e.Usage) != 1 || len(e.Signatures) != exportTestSignatures {
		t.Fatalf("export = format %q, %d members, %d invitations, %d keys, %d usage, %d signatures",
			e.Format, len(e.Members), len(e.Invitations), len(e.APIKeys), len(e.Usage), len(e.Signatures))
	}
	if e.APIKeys[0]["revoked_at"] == nil || e.APIKeys[1]["revoked_at"] != nil {
		t.Fatalf("keys = %v", e.APIKeys)
	}
	byID := map[string]map[string]any{}
	for _, o := range e.Organizations {
		byID[str(o, "id")] = o
	}
	own, delegated := byID[org], byID[theirs]
	if own == nil || delegated == nil {
		t.Fatalf("organizations = %v", e.Organizations)
	}
	if certs, _ := own["certificates"].([]any); len(certs) != 1 || own["active_certificates"].(map[string]any)["test"] == nil {
		t.Fatalf("own = %v", own)
	}
	if ds, _ := own["delegations"].([]any); len(ds) != 1 {
		t.Fatalf("own delegations = %v", own["delegations"])
	}
	// A sign-only delegation reads no certificates and lists no delegations.
	if delegated["certificates"] != nil || delegated["delegations"] != nil || delegated["relation"] != "delegated" {
		t.Fatalf("delegated = %v", delegated)
	}
}

// exportTestSignatures is more than one batch of EachSignature (1000).
const exportTestSignatures = 1003

func TestDeleteAccount(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	h.expect(h.do("GET", "/me", beto, nil), 200)

	// Ana alone in one space, with an organization, a certificate and a key.
	alone := h.space(ana, "Sola")
	org := h.organization(ana, alone, "Empresa", "0614-123456-101-2", "SV")
	h.certificate(ana, alone, org, "test", "06141234561012")
	key := h.key(ana, alone, "test", []string{"sign"}, "")
	h.expect(h.do("POST", "/signatures", key, `{"id":"d1"}`, "X-Organization-Id", org), 200)
	// The only owner of a space with Beto.
	shared := h.space(ana, "Compartido")
	if err := h.db.AddMember(ctx, shared, "beto", "member"); err != nil {
		t.Fatal(err)
	}
	// A member of Beto's.
	bs := h.space(beto, "De Beto")
	if err := h.db.AddMember(ctx, bs, "ana", "admin"); err != nil {
		t.Fatal(err)
	}

	outcomes := func() (bool, map[string]string) {
		d := h.expect(h.do("GET", "/me/account-deletion", ana, nil), 200).json(t)
		out := map[string]string{}
		for _, w := range d["workspaces"].([]any) {
			m := w.(map[string]any)
			out[str(m, "id")] = str(m, "outcome")
		}
		return d["allowed"] == true, out
	}
	allowed, o := outcomes()
	if allowed || o[alone] != "delete" || o[shared] != "transfer" || o[bs] != "leave" {
		t.Fatalf("deletion = %v %v", allowed, o)
	}
	h.expect(h.do("DELETE", "/me", ana, nil), 409, "account-has-workspaces")
	h.expect(h.do("GET", "/workspaces/"+alone, ana, nil), 200)

	// Beto made owner, the space no longer blocks; a delegation of the lone
	// space's organization to another space does.
	h.expect(h.do("PATCH", "/workspaces/"+shared+"/members/beto", ana, map[string]any{"role": "owner"}), 200)
	h.expect(h.do("POST", "/workspaces/"+alone+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": bs, "scopes": []string{"sign"}}), 201)
	if allowed, o := outcomes(); allowed || o[alone] != "delegated" || o[shared] != "leave" {
		t.Fatalf("deletion = %v %v", allowed, o)
	}
	ds := h.expect(h.do("GET", "/workspaces/"+alone+"/organizations/"+org+"/delegations", ana, nil), 200).items(t)
	h.expect(h.do("DELETE", "/workspaces/"+alone+"/organizations/"+org+"/delegations/"+str(ds[0], "id"), ana, nil), 204)
	if allowed, _ := outcomes(); !allowed {
		t.Fatal("still blocked")
	}

	h.expect(h.do("DELETE", "/me", ana, nil), 204)
	// The lone space is gone, with its key, at once; the others stay, without
	// Ana.
	h.expect(h.do("POST", "/signatures", key, `{"id":"d2"}`, "X-Organization-Id", org), 401, "unauthorized")
	var n int
	if err := h.db.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM workspaces WHERE id = $1) + (SELECT count(*) FROM organizations WHERE id = $2)
		+ (SELECT count(*) FROM workspace_members WHERE user_id = 'ana') + (SELECT count(*) FROM users WHERE id = 'ana')`, alone, org).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d rows of Ana remain", n)
	}
	for _, w := range []string{shared, bs} {
		if members := h.expect(h.do("GET", "/workspaces/"+w+"/members", beto, nil), 200).items(t); len(members) != 1 {
			t.Fatalf("members of %s = %v", w, members)
		}
	}
	// Someone in no space deletes theirs at once.
	h.expect(h.do("DELETE", "/me", h.person("dora", auth.Claims{}), nil), 204)

	// Not in the cloud.
	cloud := newHarness(t, func(d *Deps) { d.Cloud = true })
	eva := cloud.person("eva", auth.Claims{})
	cloud.expect(cloud.do("GET", "/me/account-deletion", eva, nil), 404, "not-self-hosted")
	cloud.expect(cloud.do("DELETE", "/me", eva, nil), 404, "not-self-hosted")
}
