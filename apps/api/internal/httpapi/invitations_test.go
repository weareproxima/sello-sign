package httpapi

import (
	"context"
	"sync"
	"testing"

	"proxima.la/sello/sign/internal/auth"
)

func TestInvitations(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{Name: "Ana"})
	beto := h.person("beto", auth.Claims{Name: "Beto"})
	carla := h.person("carla", auth.Claims{Name: "Carla"})
	ws := h.space(ana, "Proveedor A")
	path := "/workspaces/" + ws + "/invitations"

	// Only a person: a key never invites.
	key := h.key(ana, ws, "test", []string{"keys:read", "keys:write"}, "")
	h.expect(h.do("GET", path, key, nil), 403, "forbidden")
	h.expect(h.do("POST", path, key, map[string]any{"email": "x@example.com", "role": "member"}), 403, "forbidden")

	h.expect(h.do("POST", path, ana, map[string]any{"email": "beto", "role": "member"}), 400, "invalid-request")
	h.expect(h.do("POST", path, ana, map[string]any{"email": "beto@example.com", "role": "ADMIN"}), 400, "invalid-request")
	inv := h.expect(h.do("POST", path, ana, map[string]any{"email": " Beto@Example.com ", "role": "admin"}), 201).json(t)
	token := str(inv, "token")
	if !auth.IsInvitationToken(token) || str(inv, "email") != "beto@example.com" || str(inv, "status") != "pending" ||
		inv["invited_by"].(map[string]any)["name"] != "Ana" {
		t.Fatalf("invitation = %v", inv)
	}
	h.expect(h.do("POST", path, ana, map[string]any{"email": "beto@example.com", "role": "member"}), 409, "invitation-exists")
	list := h.expect(h.do("GET", path, ana, nil), 200).items(t)
	if len(list) != 1 || list[0]["token"] != nil || list[0]["id"] != inv["id"] {
		t.Fatalf("list = %v", list)
	}

	// Who holds the link sees it, and only the address it was sent to
	// accepts it.
	h.expect(h.do("POST", "/invitation-lookups", beto, map[string]any{"token": "sign_inv_nope"}), 404, "invitation-not-found")
	h.expect(h.do("POST", "/invitation-lookups", "", map[string]any{"token": token}), 401, "unauthorized")
	look := h.expect(h.do("POST", "/invitation-lookups", carla, map[string]any{"token": token}), 200).json(t)
	if look["workspace_id"] != ws || look["workspace_name"] != "Proveedor A" || look["role"] != "admin" ||
		look["email"] != "beto@example.com" || look["status"] != "pending" {
		t.Fatalf("lookup = %v", look)
	}
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": token}), 403, "invitation-email-mismatch")
	joined := h.expect(h.do("POST", "/invitation-acceptances", beto, map[string]any{"token": token}), 200).json(t)
	if joined["id"] != ws || joined["role"] != "admin" {
		t.Fatalf("accepted = %v", joined)
	}
	// Twice by the same account answers the same; by another, it's used.
	h.expect(h.do("POST", "/invitation-acceptances", beto, map[string]any{"token": token}), 200)
	otherBeto := h.person("beto-2", auth.Claims{Email: "BETO@example.com"})
	h.expect(h.do("POST", "/invitation-acceptances", otherBeto, map[string]any{"token": token}), 410, "invitation-used")
	if look := h.expect(h.do("POST", "/invitation-lookups", beto, map[string]any{"token": token}), 200).json(t); look["status"] != "accepted" {
		t.Fatalf("lookup after = %v", look)
	}
	if list := h.expect(h.do("GET", path, ana, nil), 200).items(t); len(list) != 0 {
		t.Fatalf("an accepted invitation is listed: %v", list)
	}
	members := h.expect(h.do("GET", "/workspaces/"+ws+"/members", ana, nil), 200).items(t)
	if len(members) != 2 || members[1]["user_id"] != "beto" || members[1]["role"] != "admin" || members[1]["name"] != "Beto" {
		t.Fatalf("members = %v", members)
	}
	h.expect(h.do("POST", path, ana, map[string]any{"email": "beto@example.com", "role": "member"}), 409, "already-member")

	// An admin invites, but not an owner.
	h.expect(h.do("POST", path, beto, map[string]any{"email": "carla@example.com", "role": "owner"}), 403, "forbidden")
	c := h.expect(h.do("POST", path, beto, map[string]any{"email": "carla@example.com", "role": "member"}), 201).json(t)
	// Revoked, its link stops working.
	h.expect(h.do("DELETE", path+"/"+str(c, "id"), ana, nil), 204)
	h.expect(h.do("DELETE", path+"/"+str(c, "id"), ana, nil), 404, "invitation-not-found")
	h.expect(h.do("DELETE", path+"/nope", ana, nil), 404, "invitation-not-found")
	h.expect(h.do("POST", "/invitation-lookups", carla, map[string]any{"token": str(c, "token")}), 404, "invitation-not-found")
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": str(c, "token")}), 404, "invitation-not-found")

	// Expired: listed as such, not accepted, and replaced by a new one.
	c = h.expect(h.do("POST", path, ana, map[string]any{"email": "carla@example.com", "role": "member"}), 201).json(t)
	if _, err := h.db.Pool().Exec(ctx, `UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE id = $1`, str(c, "id")); err != nil {
		t.Fatal(err)
	}
	if list := h.expect(h.do("GET", path, ana, nil), 200).items(t); len(list) != 1 || list[0]["status"] != "expired" {
		t.Fatalf("expired = %v", list)
	}
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": str(c, "token")}), 410, "invitation-expired")
	again := h.expect(h.do("POST", path, ana, map[string]any{"email": "carla@example.com", "role": "member"}), 201).json(t)
	if list := h.expect(h.do("GET", path, ana, nil), 200).items(t); len(list) != 1 || list[0]["id"] != again["id"] {
		t.Fatalf("not replaced: %v", list)
	}
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": str(again, "token")}), 200)

	// A member sees no invitations and changes no roles.
	h.expect(h.do("GET", path, carla, nil), 403, "forbidden")
	h.expect(h.do("POST", path, carla, map[string]any{"email": "d@example.com", "role": "member"}), 403, "forbidden")
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/members/beto", carla, map[string]any{"role": "member"}), 403, "forbidden")
}

func TestMemberRoles(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})
	ws := h.space(ana, "Proveedor A")
	if err := h.db.AddMember(ctx, ws, "beto", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := h.db.AddMember(ctx, ws, "carla", "member"); err != nil {
		t.Fatal(err)
	}
	m := func(user string) string { return "/workspaces/" + ws + "/members/" + user }

	h.expect(h.do("PATCH", m("carla"), ana, map[string]any{"role": "boss"}), 400, "invalid-request")
	h.expect(h.do("PATCH", m("nadie"), ana, map[string]any{"role": "member"}), 404, "member-not-found")
	// An admin moves admins and members, never an owner.
	if r := h.expect(h.do("PATCH", m("carla"), beto, map[string]any{"role": "admin"}), 200).json(t); r["role"] != "admin" || r["user_id"] != "carla" {
		t.Fatalf("changed = %v", r)
	}
	h.expect(h.do("PATCH", m("carla"), beto, map[string]any{"role": "owner"}), 403, "forbidden")
	h.expect(h.do("PATCH", m("ana"), beto, map[string]any{"role": "member"}), 403, "forbidden")
	// The space never loses its last owner.
	h.expect(h.do("PATCH", m("ana"), ana, map[string]any{"role": "admin"}), 409, "last-owner")
	h.expect(h.do("PATCH", m("ana"), ana, map[string]any{"role": "owner"}), 200)
	h.expect(h.do("PATCH", m("beto"), ana, map[string]any{"role": "owner"}), 200)
	h.expect(h.do("PATCH", m("ana"), ana, map[string]any{"role": "member"}), 200)
	h.expect(h.do("PATCH", m("beto"), beto, map[string]any{"role": "admin"}), 409, "last-owner")
	h.expect(h.do("PATCH", m("carla"), ana, map[string]any{"role": "member"}), 403, "forbidden")

	// Two owners demoting each other at once: one stays.
	h.expect(h.do("PATCH", m("ana"), beto, map[string]any{"role": "owner"}), 200)
	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i, pair := range [][2]string{{ana, "beto"}, {beto, "ana"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = h.do("PATCH", m(pair[1]), pair[0], map[string]any{"role": "admin"}).status
		}()
	}
	wg.Wait()
	var owners int
	if err := h.db.Pool().QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'owner'`, ws).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 {
		t.Fatalf("owners after demoting each other: %d (%v)", owners, statuses)
	}
}

func TestInvitationsLimits(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{})
	beto := h.person("beto", auth.Claims{})

	// A plan's members: inviting and accepting both stop at it.
	ws := h.space(ana, "Pequeño")
	inv := h.expect(h.do("POST", "/workspaces/"+ws+"/invitations", ana, map[string]any{"email": "beto@example.com", "role": "member"}), 201).json(t)
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET max_members = 1 WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/workspaces/"+ws+"/invitations", ana, map[string]any{"email": "carla@example.com", "role": "member"}), 409, "workspace-full")
	h.expect(h.do("POST", "/invitation-acceptances", beto, map[string]any{"token": str(inv, "token")}), 409, "workspace-full")

	// A space of an organization of Sello's IdP: its members are billing's.
	linked := h.space(ana, "De Sello")
	inv = h.expect(h.do("POST", "/workspaces/"+linked+"/invitations", ana, map[string]any{"email": "beto@example.com", "role": "member"}), 201).json(t)
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET sello_workspace_id = 'idp-x' WHERE id = $1`, linked); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/workspaces/"+linked+"/invitations", ana, map[string]any{"email": "carla@example.com", "role": "member"}), 409, "members-managed-by-sello")
	h.expect(h.do("PATCH", "/workspaces/"+linked+"/members/ana", ana, map[string]any{"role": "admin"}), 409, "members-managed-by-sello")
	h.expect(h.do("DELETE", "/workspaces/"+linked+"/invitations/"+str(inv, "id"), ana, nil), 409, "members-managed-by-sello")
	h.expect(h.do("POST", "/invitation-acceptances", beto, map[string]any{"token": str(inv, "token")}), 409, "members-managed-by-sello")
}
