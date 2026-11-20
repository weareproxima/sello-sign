package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/retention"
	"proxima.la/sello/sign/internal/store"
)

// auditEvent is an event as GET /workspaces/{w}/audit-events gives it.
type auditEvent struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Actor  struct {
		Type  string  `json:"type"`
		ID    *string `json:"id"`
		Email *string `json:"email"`
		Name  *string `json:"name"`
		Via   *string `json:"via"`
	} `json:"actor"`
	Target struct {
		Type  string  `json:"type"`
		ID    *string `json:"id"`
		Label string  `json:"label"`
	} `json:"target"`
	Details   map[string]any `json:"details"`
	CreatedAt time.Time      `json:"created_at"`
}

func (e auditEvent) actorID() string {
	if e.Actor.ID == nil {
		return ""
	}
	return *e.Actor.ID
}

func (e auditEvent) targetID() string {
	if e.Target.ID == nil {
		return ""
	}
	return *e.Target.ID
}

// events reads a space's whole audit log as token, page by page (of
// three, to go through the cursor), newest first, with query's filters.
func (h *harness) events(token, ws, query string) []auditEvent {
	h.t.Helper()
	var out []auditEvent
	cursor := ""
	for {
		q, _ := url.ParseQuery(query)
		q.Set("limit", "3")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		r := h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?"+q.Encode(), token, nil), 200)
		var page struct {
			Items      []auditEvent `json:"items"`
			NextCursor *string      `json:"next_cursor"`
		}
		if err := json.Unmarshal(r.body, &page); err != nil {
			h.t.Fatalf("not a page: %s", r.body)
		}
		out = append(out, page.Items...)
		if page.NextCursor == nil {
			return out
		}
		cursor = *page.NextCursor
	}
}

// only is the events of action, oldest first.
func only(events []auditEvent, action audit.Action) []auditEvent {
	var out []auditEvent
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Action == string(action) {
			out = append(out, events[i])
		}
	}
	return out
}

// one is the single event of action, failing if there isn't exactly one.
func one(t *testing.T, events []auditEvent, action audit.Action) auditEvent {
	t.Helper()
	got := only(events, action)
	if len(got) != 1 {
		t.Fatalf("%d events %s, want 1: %+v", len(got), action, got)
	}
	return got[0]
}

func changes(t *testing.T, e auditEvent) map[string]any {
	t.Helper()
	c, ok := e.Details["changes"].(map[string]any)
	if !ok {
		t.Fatalf("%s without changes: %v", e.Action, e.Details)
	}
	return c
}

func change(t *testing.T, e auditEvent, field string) (from, to any) {
	t.Helper()
	c, ok := changes(t, e)[field].(map[string]any)
	if !ok {
		t.Fatalf("%s doesn't change %s: %v", e.Action, field, e.Details)
	}
	return c["from"], c["to"]
}

// Each change to a space and what it holds is recorded, with who made it,
// in the space's audit log, which only its owners and admins read.
func TestAuditLog(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{Name: "Ana Pérez", Email: "ana@example.com"})
	beto := h.person("beto", auth.Claims{Name: "Beto"})
	carla := h.person("carla", auth.Claims{Name: "Carla", Email: "carla@example.com"})
	dora := h.person("dora", auth.Claims{Email: "dora@example.com"})

	// The space and its settings.
	ws := h.space(ana, "Proveedor A")
	h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"name": "Proveedor B", "requests_per_minute": 30}), 200)
	// A PATCH that changes nothing records nothing.
	h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"name": "Proveedor B"}), 200)

	// An organization, its certificates and a delegation.
	org := h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV")
	h.organization(ana, ws, "Empresa", "0614-123456-101-2", "SV") // the same one: nothing new
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/organizations/"+org, ana,
		map[string]any{"name": "Empresa S.A.", "contact_email": "facturas@empresa.test"}), 200)
	first := h.certificate(ana, ws, org, "test", "06141234561012")
	second := h.certificate(ana, ws, org, "test", "06141234561012")
	certPath := "/workspaces/" + ws + "/organizations/" + org + "/certificates/"
	h.expect(h.do("PATCH", certPath+second, ana, map[string]any{"active": false}), 200)
	h.expect(h.do("PATCH", certPath+second, ana, map[string]any{"active": false}), 200) // already: nothing
	h.expect(h.do("PATCH", certPath+first, ana, map[string]any{"active": true}), 200)
	h.expect(h.do("DELETE", certPath+second, ana, nil), 204)

	other := h.space(beto, "Otro")
	delegationPath := "/workspaces/" + ws + "/organizations/" + org + "/delegations"
	d1 := str(h.expect(h.do("POST", delegationPath, ana, map[string]any{"workspace_id": other, "scopes": []string{"sign"}}), 201).json(t), "id")
	h.expect(h.do("DELETE", delegationPath+"/"+d1, ana, nil), 204)
	d2 := str(h.expect(h.do("POST", delegationPath, ana,
		map[string]any{"workspace_id": other, "scopes": []string{"sign", "certificates:write"}}), 201).json(t), "id")
	// The delegate uploads a certificate to Ana's organization, and then
	// gives the delegation up.
	h.certificate(beto, other, org, "prod", "06141234561012")
	h.expect(h.do("DELETE", "/workspaces/"+other+"/organizations/"+org+"/delegations/"+d2, beto, nil), 204)

	// Keys: by a person, by a key, and revoked through the MCP server.
	admin := h.key(ana, ws, "test", []string{"sign", "keys:read", "keys:write", "organizations:write"}, "")
	h.expect(h.do("POST", "/workspaces/self/api-keys", admin, map[string]any{
		"name": "Hecha por una clave", "environment": "test", "scopes": []string{"sign"}, "requests_per_minute": 5,
	}), 201)
	keys := h.expect(h.do("GET", "/workspaces/self/api-keys", admin, nil), 200).items(t)
	byKey := str(keys[0], "id")
	if text, isErr := toolText(t, h.rpc(admin, callTool("revoke_api_key", `{"api_key_id":"`+byKey+`"}`)).result(t)); isErr {
		t.Fatalf("revoke_api_key = %s", text)
	}
	// A key of only one organization goes with it.
	onlyKey := h.key(ana, ws, "test", []string{"sign"}, org)

	// Invitations and members.
	invPath := "/workspaces/" + ws + "/invitations"
	toCarla := h.expect(h.do("POST", invPath, ana, map[string]any{"email": "carla@example.com", "role": "admin"}), 201).json(t)
	renewed := h.expect(h.do("POST", invPath+"/"+str(toCarla, "id")+"/renewals", ana, nil), 200).json(t)
	h.expect(h.do("POST", "/invitation-acceptances", carla, map[string]any{"token": str(renewed, "token")}), 200)
	toDora := h.expect(h.do("POST", invPath, carla, map[string]any{"email": "dora@example.com", "role": "member"}), 201).json(t)
	h.expect(h.do("DELETE", invPath+"/"+str(toDora, "id"), carla, nil), 204)
	toDora = h.expect(h.do("POST", invPath, carla, map[string]any{"email": "dora@example.com", "role": "member"}), 201).json(t)
	h.expect(h.do("POST", "/invitation-acceptances", dora, map[string]any{"token": str(toDora, "token")}), 200)
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/members/dora", ana, map[string]any{"role": "admin"}), 200)
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/members/dora", ana, map[string]any{"role": "admin"}), 200) // the same: nothing

	// Who reads it: owners and admins, with the token.
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events", carla, nil), 200)
	h.expect(h.do("PATCH", "/workspaces/"+ws+"/members/carla", ana, map[string]any{"role": "member"}), 200)
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events", carla, nil), 403, "forbidden")
	h.expect(h.do("GET", "/workspaces/self/audit-events", admin, nil), 403, "forbidden")
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events", beto, nil), 404, "workspace-not-found")
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?action=nope", ana, nil), 400, "invalid-request")
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?target_type=nope", ana, nil), 400, "invalid-request")
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?actor_type=nope", ana, nil), 400, "invalid-request")
	h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?from=ayer", ana, nil), 400, "invalid-request")

	h.expect(h.do("DELETE", "/workspaces/"+ws+"/members/carla", ana, nil), 204)
	h.expect(h.do("DELETE", "/workspaces/"+ws+"/members/dora", dora, nil), 204)

	// The organization goes, with its key of only it.
	h.expect(h.do("DELETE", "/workspaces/self/organizations/"+org, admin, nil), 204)
	h.expect(h.do("POST", "/signatures", onlyKey, `{"id":"x"}`, "X-Organization-Id", org), 401, "unauthorized")

	// The console's changes are the console's.
	console := audit.WithActor(ctx, audit.Actor{Type: audit.ActorConsole})
	if err := h.db.AddMember(console, ws, "beto", "member"); err != nil {
		t.Fatal(err)
	}
	ten := 10
	if _, err := h.db.SetLimits(console, ws, store.LimitsPatch{RetentionDays: limitPtr(&ten)}); err != nil {
		t.Fatal(err)
	}

	export := h.expect(h.do("GET", "/workspaces/"+ws+"/export", ana, nil), 200)

	all := h.events(ana, ws, "")
	if len(all) < 30 {
		t.Fatalf("%d events", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].ID >= all[i-1].ID || all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Fatalf("not newest first at %d: %+v", i, all[i])
		}
	}
	for _, e := range all {
		if e.Actor.Type == "user" && e.actorID() == "ana" && (e.Actor.Email == nil || *e.Actor.Email != "ana@example.com" || *e.Actor.Name != "Ana Pérez") {
			t.Fatalf("Ana as the actor = %+v", e.Actor)
		}
	}

	created := one(t, all, audit.WorkspaceCreated)
	if created.actorID() != "ana" || created.targetID() != ws || created.Target.Label != "Proveedor A" || created.Details["default_country"] != "SV" {
		t.Fatalf("created = %+v", created)
	}
	updated := one(t, all, audit.WorkspaceUpdated)
	if from, to := change(t, updated, "name"); from != "Proveedor A" || to != "Proveedor B" {
		t.Fatalf("name %v → %v", from, to)
	}
	if from, to := change(t, updated, "requests_per_minute"); from != nil || to != float64(30) {
		t.Fatalf("rate %v → %v", from, to)
	}
	if _, ok := changes(t, updated)["default_country"]; ok {
		t.Fatalf("an unchanged field: %v", updated.Details)
	}

	orgCreated := one(t, all, audit.OrganizationCreated)
	if orgCreated.targetID() != org || orgCreated.Target.Label != "Empresa" || orgCreated.Details["tax_id"] != "0614-123456-101-2" {
		t.Fatalf("organization created = %+v", orgCreated)
	}
	orgUpdated := one(t, all, audit.OrganizationUpdated)
	if from, to := change(t, orgUpdated, "contact_email"); from != nil || to != "facturas@empresa.test" || orgUpdated.Target.Label != "Empresa S.A." {
		t.Fatalf("organization updated = %+v", orgUpdated)
	}

	uploads := only(all, audit.CertificateUploaded)
	if len(uploads) != 3 {
		t.Fatalf("uploads = %+v", uploads)
	}
	if uploads[0].targetID() != first || uploads[0].Details["environment"] != "test" || uploads[0].Details["serial"] != "01" ||
		uploads[0].Details["replaced_certificate_id"] != nil || uploads[0].Target.Label != "Empresa S.A." {
		t.Fatalf("first upload = %+v", uploads[0])
	}
	if uploads[1].targetID() != second || uploads[1].Details["replaced_certificate_id"] != first {
		t.Fatalf("second upload = %+v", uploads[1])
	}
	// The delegate's upload, in the owner's log, is the delegate space's.
	if uploads[2].Actor.Type != "workspace" || uploads[2].actorID() != other || *uploads[2].Actor.Name != "Otro" ||
		uploads[2].Actor.Email != nil || uploads[2].Details["environment"] != "prod" {
		t.Fatalf("the delegate's upload = %+v", uploads[2])
	}
	if d := one(t, all, audit.CertificateDeactivated); d.targetID() != second {
		t.Fatalf("deactivated = %+v", d)
	}
	if a := one(t, all, audit.CertificateActivated); a.targetID() != first || a.Details["replaced_certificate_id"] != nil {
		t.Fatalf("activated = %+v", a)
	}
	if d := one(t, all, audit.CertificateDeleted); d.targetID() != second || d.Details["active"] != false {
		t.Fatalf("certificate deleted = %+v", d)
	}

	granted := only(all, audit.DelegationGranted)
	removed := only(all, audit.DelegationRemoved)
	if len(granted) != 2 || len(removed) != 2 {
		t.Fatalf("delegations: %d granted, %d removed", len(granted), len(removed))
	}
	if g := granted[0]; g.targetID() != d1 || g.actorID() != "ana" || g.Details["workspace_id"] != other ||
		g.Details["workspace_name"] != "Otro" || g.Target.Label != "Empresa S.A." {
		t.Fatalf("granted = %+v", g)
	}
	if r := removed[0]; r.targetID() != d1 || r.actorID() != "ana" || r.Details["renounced"] != false {
		t.Fatalf("removed = %+v", r)
	}
	// Given up by the delegate: in the owner's log, the delegate space.
	if r := removed[1]; r.targetID() != d2 || r.Actor.Type != "workspace" || r.actorID() != other || r.Details["renounced"] != true {
		t.Fatalf("renounced = %+v", r)
	}

	keyEvents := only(all, audit.APIKeyCreated)
	if len(keyEvents) != 3 {
		t.Fatalf("keys created = %+v", keyEvents)
	}
	byAKey := keyEvents[1]
	if byAKey.Actor.Type != "api_key" || byAKey.Actor.Name == nil || *byAKey.Actor.Name != "k" || byAKey.Actor.Via != nil ||
		byAKey.Target.Label != "Hecha por una clave" || byAKey.Details["requests_per_minute"] != float64(5) {
		t.Fatalf("a key's key = %+v", byAKey)
	}
	revokes := only(all, audit.APIKeyRevoked)
	if len(revokes) != 2 {
		t.Fatalf("revoked = %+v", revokes)
	}
	if r := revokes[0]; r.targetID() != byKey || r.Actor.Type != "api_key" || r.Actor.Via == nil || *r.Actor.Via != "mcp" {
		t.Fatalf("revoked through MCP = %+v", r)
	}
	if r := revokes[1]; r.Details["reason"] != "organization_deleted" || r.Actor.Type != "api_key" {
		t.Fatalf("revoked with its organization = %+v", r)
	}
	if d := one(t, all, audit.OrganizationDeleted); d.targetID() != org || d.Actor.Type != "api_key" || d.Details["certificates"] != float64(2) {
		t.Fatalf("organization deleted = %+v", d)
	}

	invites := only(all, audit.InvitationCreated)
	if len(invites) != 3 || invites[0].Target.Label != "carla@example.com" || invites[0].Details["role"] != "admin" ||
		invites[1].actorID() != "carla" {
		t.Fatalf("invitations = %+v", invites)
	}
	if r := one(t, all, audit.InvitationRenewed); r.targetID() != str(toCarla, "id") {
		t.Fatalf("renewed = %+v", r)
	}
	if r := one(t, all, audit.InvitationRevoked); r.Target.Label != "dora@example.com" {
		t.Fatalf("revoked = %+v", r)
	}
	accepted := only(all, audit.InvitationAccepted)
	if len(accepted) != 2 || accepted[0].actorID() != "carla" || accepted[0].Details["joined"] != true || accepted[1].actorID() != "dora" {
		t.Fatalf("accepted = %+v", accepted)
	}
	roles := only(all, audit.MemberRoleChanged)
	if len(roles) != 2 || roles[0].targetID() != "dora" || roles[0].Target.Label != "dora@example.com" ||
		roles[0].Details["from"] != "member" || roles[0].Details["to"] != "admin" || roles[1].Details["to"] != "member" {
		t.Fatalf("roles = %+v", roles)
	}
	if r := one(t, all, audit.MemberRemoved); r.targetID() != "carla" || r.actorID() != "ana" || r.Details["role"] != "member" {
		t.Fatalf("removed = %+v", r)
	}
	if l := one(t, all, audit.MemberLeft); l.targetID() != "dora" || l.actorID() != "dora" {
		t.Fatalf("left = %+v", l)
	}
	if a := one(t, all, audit.MemberAdded); a.Actor.Type != "console" || a.Actor.ID != nil || a.targetID() != "beto" {
		t.Fatalf("added = %+v", a)
	}
	if l := one(t, all, audit.WorkspaceLimitsChanged); l.Actor.Type != "console" {
		t.Fatalf("limits = %+v", l)
	} else if from, to := change(t, l, "retention_days"); from != nil || to != float64(10) {
		t.Fatalf("retention %v → %v", from, to)
	}
	if e := one(t, all, audit.WorkspaceExported); e.actorID() != "ana" || all[0].Action != string(audit.WorkspaceExported) {
		t.Fatalf("exported = %+v", e)
	}

	// The delegate space's log: the delegations, by Ana's space, and what
	// it did itself; never Ana.
	theirs := h.events(beto, other, "")
	if g := only(theirs, audit.DelegationGranted); len(g) != 2 || g[0].Actor.Type != "workspace" || g[0].actorID() != ws {
		t.Fatalf("the delegate's grants = %+v", g)
	}
	if u := one(t, theirs, audit.CertificateUploaded); u.actorID() != "beto" {
		t.Fatalf("the delegate's upload = %+v", u)
	}
	if r := only(theirs, audit.DelegationRemoved); len(r) != 2 || r[0].actorID() != ws || r[1].actorID() != "beto" {
		t.Fatalf("the delegate's removals = %+v", r)
	}

	// Filters.
	for query, want := range map[string]int{
		"action=certificate.uploaded": 3,
		"target_type=invitation":      7,
		"actor_type=api_key":          4,
		"actor_type=workspace":        2,
		"actor_id=carla":              4,
		"target_id=" + org:            3,
		"actor_type=console":          2,
		"from=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339): 0,
		"to=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339):   len(all),
	} {
		if got := h.events(ana, ws, query); len(got) != want {
			t.Errorf("%s: %d events, want %d", query, len(got), want)
		}
	}

	// Never a secret.
	raw := h.expect(h.do("GET", "/workspaces/"+ws+"/audit-events?limit=100", ana, nil), 200).body
	for _, secret := range []string{admin, onlyKey, str(toCarla, "token"), str(renewed, "token"), str(toDora, "token"),
		fakeCert("06141234561012"), "hash", "material", "password"} {
		if strings.Contains(string(raw), secret) || strings.Contains(string(export.body), secret) {
			t.Fatalf("an event carries %q", secret)
		}
	}

	// The export has the log, newest first, starting with itself.
	var e struct {
		AuditEvents []auditEvent `json:"audit_events"`
	}
	if err := json.Unmarshal(export.body, &e); err != nil {
		t.Fatal(err)
	}
	if len(e.AuditEvents) != len(all) || e.AuditEvents[0].Action != string(audit.WorkspaceExported) {
		t.Fatalf("the export's log: %d events, first %+v", len(e.AuditEvents), e.AuditEvents[0])
	}
}

func limitPtr(p *int) **int { return &p }

// In the cloud: what billing pushes, the organizations a company brings
// from the platform's space and gives back, and the platform's locked
// delegation, each in both spaces.
func TestAuditCloud(t *testing.T) {
	h := newHarness(t, func(d *Deps) {
		d.Cloud = true
		d.BillingPushToken = "push-token"
	})
	ctx := context.Background()
	ps, err := h.db.CreateWorkspace(ctx, "Sello", nil, "staff")
	if err != nil {
		t.Fatal(err)
	}
	platform := ps.ID
	h.server.PlatformWorkspace = platform
	staff := h.person("staff", auth.Claims{Email: "staff@sello.tax"})
	platformKey := h.key(staff, platform, "test", []string{"sign", "organizations:write", "organizations:read", "certificates:write"}, "")
	oc := str(h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Empresa Oc", "tax_id": "0614-1", "country": "SV", "sello_workspace_id": "idp-oc"}), 201).json(t), "id")

	// The company's space, as billing opened it.
	admin := h.person("u-admin", auth.Claims{Email: "admin@oc.test", SelloSub: "s-admin", SelloWorkspaces: []auth.SelloWorkspace{
		{ID: "idp-oc", Name: "Empresa Oc", Country: "SV", Role: "owner"},
	}})
	ws, err := h.db.CreateWorkspace(ctx, "Empresa Oc", nil, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET sello_workspace_id = 'idp-oc-space' WHERE id = $1`, ws.ID); err != nil {
		t.Fatal(err)
	}
	push := func(version int, closed bool, plan string, prod int) {
		t.Helper()
		body := `{"workspace_id":"idp-oc-space","version":` + strconv.Itoa(version) + `,"closed":` + map[bool]string{true: "true", false: "false"}[closed] +
			`,"name":"Empresa Oc","members":[{"account":"s-admin","role":"OWNER"}],"plan":"` + plan +
			`","limits":{"issuers":5,"prod_signatures":` + strconv.Itoa(prod) + `,"test_signatures":null,"members":3,"requests_per_minute":60,"retention_days":30},"hard_caps":{"prod":null,"test":null}}`
		h.expect(h.do("PUT", "/api/billing/organizations/idp-oc-space", "push-token", body), 204)
	}
	push(1, false, "PRO_STARTER", 1000)
	push(1, false, "PRO_GROWTH", 5000) // stale: nothing
	push(2, false, "PRO_GROWTH", 5000)
	push(3, false, "PRO_GROWTH", 5000) // the same state: nothing

	// It brings the organization from the platform's space.
	h.expect(h.do("POST", "/workspaces/"+ws.ID+"/organization-claims", admin, map[string]any{"organization_ids": []string{oc}}), 200)
	// The platform uploads a certificate to it with its locked delegation.
	h.certificate(platformKey, "self", oc, "test", "06141")
	// And billing closes the space: the organization goes back.
	push(4, true, "PRO_GROWTH", 5000)
	push(5, false, "PRO_GROWTH", 5000)

	mine := h.events(admin, ws.ID, "")
	plans := only(mine, audit.WorkspacePlanChanged)
	if len(plans) != 2 {
		t.Fatalf("plan changes = %+v", plans)
	}
	if p := plans[1]; p.Actor.Type != "billing" || p.Actor.ID != nil {
		t.Fatalf("plan changed = %+v", p)
	} else if from, to := change(t, p, "plan"); from != "pro_starter" || to != "pro_growth" {
		t.Fatalf("plan %v → %v", from, to)
	} else if from, to := change(t, p, "prod_signatures"); from != float64(1000) || to != float64(5000) {
		t.Fatalf("prod %v → %v", from, to)
	}
	if c := one(t, mine, audit.WorkspaceClosed); c.Actor.Type != "billing" {
		t.Fatalf("closed = %+v", c)
	}
	one(t, mine, audit.WorkspaceReopened)
	if c := one(t, mine, audit.OrganizationClaimed); c.actorID() != "u-admin" || c.Details["from_workspace_id"] != platform {
		t.Fatalf("claimed = %+v", c)
	}
	if u := one(t, mine, audit.CertificateUploaded); u.Actor.Type != "workspace" || u.actorID() != platform || *u.Actor.Name != "Sello" {
		t.Fatalf("the platform's upload, in the company's log = %+v", u)
	}
	if r := one(t, mine, audit.OrganizationReturned); r.Actor.Type != "billing" || r.targetID() != oc {
		t.Fatalf("returned = %+v", r)
	}

	theirs := h.events(staff, platform, "")
	if c := one(t, theirs, audit.OrganizationClaimed); c.Actor.Type != "workspace" || c.actorID() != ws.ID || c.Actor.Email != nil {
		t.Fatalf("claimed, in the platform's log = %+v", c)
	}
	if u := one(t, theirs, audit.CertificateUploaded); u.Actor.Type != "api_key" {
		t.Fatalf("the platform's own upload = %+v", u)
	}
	if r := one(t, theirs, audit.OrganizationReturned); r.Actor.Type != "billing" || r.Details["from_workspace_id"] != ws.ID {
		t.Fatalf("returned, in the platform's log = %+v", r)
	}

	// The platform registers an organization a company had already: the
	// company's space gets the platform's locked delegation.
	company, err := h.db.CreateWorkspace(ctx, "Otra empresa", nil, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	theirOrg := h.organization(admin, company.ID, "Lácteos", "0614-2", "SV")
	if _, err := h.db.Pool().Exec(ctx, `UPDATE organizations SET sello_workspace_id = 'idp-lacteos' WHERE id = $1`, theirOrg); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Lácteos", "tax_id": "0614-2", "country": "SV", "sello_workspace_id": "idp-lacteos"}), 200)
	h.expect(h.do("POST", "/workspaces/self/organizations", platformKey,
		map[string]any{"name": "Lácteos", "tax_id": "0614-2", "country": "SV", "sello_workspace_id": "idp-lacteos"}), 200)
	if g := one(t, h.events(admin, company.ID, ""), audit.DelegationGranted); g.Actor.Type != "workspace" || g.actorID() != platform ||
		g.Details["locked"] != true || g.Details["workspace_id"] != platform {
		t.Fatalf("the platform's delegation, in the company's log = %+v", g)
	}
	if g := one(t, h.events(staff, platform, ""), audit.DelegationGranted); g.Actor.Type != "api_key" {
		t.Fatalf("the platform's delegation, in its own log = %+v", g)
	}
}

// What's older than the retention goes, whatever the plan, with the
// signing log's purge; and what's left goes with its space or, for a
// person who deletes their account, stays in the spaces they leave.
func TestAuditRetentionAndDeletion(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana := h.person("ana", auth.Claims{Email: "ana@example.com"})
	beto := h.person("beto", auth.Claims{Email: "beto@example.com"})
	ws := h.space(ana, "Proveedor A")
	other := h.space(beto, "De Beto")
	inv := h.expect(h.do("POST", "/workspaces/"+other+"/invitations", beto, map[string]any{"email": "ana@example.com", "role": "member"}), 201).json(t)
	h.expect(h.do("POST", "/invitation-acceptances", ana, map[string]any{"token": str(inv, "token")}), 200)

	now := time.Now().UTC()
	old := now.Add(-audit.Retention - time.Hour)
	recent := now.Add(-audit.Retention + time.Hour)
	for i, at := range []time.Time{old, old.Add(time.Minute), recent} {
		// An id of that instant (the store's are the process's, never older
		// than its last one).
		floor := ids.Floor(at)
		id := floor[:14] + "7000-8000-" + fmt.Sprintf("%012x", i)
		if _, err := h.db.Pool().Exec(ctx, `INSERT INTO audit_events (id, workspace_id, actor_type, action, target_type, created_at)
			VALUES ($1, $2, 'system', 'workspace.updated', 'workspace', $3)`, id, []string{ws, other, ws}[i], at); err != nil {
			t.Fatal(err)
		}
	}
	before := len(h.events(ana, ws, ""))
	ran, purged, err := retention.Once(ctx, h.db, now)
	if !ran || err != nil || purged.AuditEvents != 2 {
		t.Fatalf("purge: ran %v purged %+v %v", ran, purged, err)
	}
	if after := len(h.events(ana, ws, "")); after != before-1 {
		t.Fatalf("%d events before, %d after", before, after)
	}

	// Ana deletes her account: her space goes with its log; she leaves
	// Beto's, which keeps what she did there.
	h.expect(h.do("DELETE", "/me", ana, nil), 204)
	var n int
	if err := h.db.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE workspace_id = $1`, ws).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d events of the deleted space (%v)", n, err)
	}
	theirs := h.events(beto, other, "")
	if a := one(t, theirs, audit.InvitationAccepted); a.actorID() != "ana" || *a.Actor.Email != "ana@example.com" {
		t.Fatalf("accepted = %+v", a)
	}
	if l := one(t, theirs, audit.MemberLeft); l.targetID() != "ana" || l.Target.Label != "ana@example.com" || l.Details["account_deleted"] != true {
		t.Fatalf("left = %+v", l)
	}
}
