package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/retention"
	"proxima.la/sello/sign/internal/store"
)

// clock is a test's time, which it moves by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func setLimits(t *testing.T, db *store.Store, ws string, p store.LimitsPatch) {
	t.Helper()
	if _, err := db.SetLimits(context.Background(), ws, p); err != nil {
		t.Fatal(err)
	}
}

func limit(n int) **int {
	p := &n
	return &p
}

func problemOf(t *testing.T, r response) Problem {
	t.Helper()
	var p Problem
	if err := json.Unmarshal(r.body, &p); err != nil {
		t.Fatalf("not a problem: %s", r.body)
	}
	return p
}

func TestMonthlyLimit(t *testing.T) {
	clk := &clock{now: time.Now().UTC().Truncate(time.Hour)}
	h := newHarness(t, func(d *Deps) { d.Now = clk.Now })
	ana, carl := h.person("ana", auth.Claims{}), h.person("carl", auth.Claims{})
	owner := h.space(ana, "Dueño")
	org := h.organization(ana, owner, "Empresa", "0614", "SV")
	h.certificate(ana, owner, org, "prod", "0614")
	h.certificate(ana, owner, org, "test", "0614")
	prodKey := h.key(ana, owner, "prod", []string{"sign"}, "")
	testKey := h.key(ana, owner, "test", []string{"sign"}, "")

	// A self-hosted space without limits signs as much as it wants.
	for i := range 3 {
		h.expect(h.do("POST", "/signatures", prodKey, fmt.Sprintf(`{"id":"n%d"}`, i), "X-Organization-Id", org), 200)
	}
	// The console sets two in production; it has signed three.
	setLimits(t, h.db, owner, store.LimitsPatch{ProdSignatures: limit(2)})
	r := h.expect(h.do("POST", "/signatures", prodKey, `{"id":"n4"}`, "X-Organization-Id", org), 429, "monthly-limit-reached")
	p := problemOf(t, r)
	if p.Limit == nil || *p.Limit != 2 || p.Environment != model.Prod || p.ResetsAt == nil || r.header.Get("Retry-After") == "" {
		t.Fatalf("problem = %s, Retry-After %q", r.body, r.header.Get("Retry-After"))
	}
	if !p.ResetsAt.Equal(nextMonth(clk.Now())) {
		t.Fatalf("resets at %v", p.ResetsAt)
	}
	// Nor from the panel; the test environment is apart.
	h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/signatures?environment=prod", ana, `{"id":"n5"}`), 429, "monthly-limit-reached")
	h.expect(h.do("POST", "/signatures", testKey, `{"id":"n6"}`, "X-Organization-Id", org), 200)

	// A space that signs with the organization by delegation counts in
	// its own space: the owner's cap doesn't stop it, and its own does.
	erp := h.space(carl, "ERP")
	h.expect(h.do("POST", "/workspaces/"+owner+"/organizations/"+org+"/delegations", ana,
		map[string]any{"workspace_id": erp, "scopes": []string{"sign"}}), 201)
	erpKey := h.key(carl, erp, "prod", []string{"sign"}, "")
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"n7"}`, "X-Organization-Id", org), 200)
	setLimits(t, h.db, erp, store.LimitsPatch{ProdSignatures: limit(1)})
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"n8"}`, "X-Organization-Id", org), 429, "monthly-limit-reached")

	// The cloud's paid plan: past the quota (one), up to the hard cap
	// (two): the overage that billing charges.
	ctx := context.Background()
	if _, err := h.db.Pool().Exec(ctx, `UPDATE workspaces SET plan = 'pro_starter', max_prod_signatures = 1,
		prod_hard_cap = 2 WHERE id = $1`, erp); err != nil {
		t.Fatal(err)
	}
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"n9"}`, "X-Organization-Id", org), 200)
	h.expect(h.do("POST", "/signatures", erpKey, `{"id":"n10"}`, "X-Organization-Id", org), 429, "monthly-limit-reached")

	// The month's count is the counters' sum, whoever writes them.
	if n, err := h.db.MonthUsage(ctx, erp, model.Prod, model.Month(clk.Now())); err != nil || n != 2 {
		t.Fatalf("erp's month = %d %v", n, err)
	}
	if _, err := h.db.Pool().Exec(ctx, `DELETE FROM usage_counters WHERE workspace_id = $1`, erp); err != nil {
		t.Fatal(err)
	}
	if n, _ := h.db.MonthUsage(ctx, erp, model.Prod, model.Month(clk.Now())); n != 0 {
		t.Fatalf("after deleting its counters, erp's month = %d", n)
	}

	// The next month starts from zero.
	clk.set(nextMonth(clk.Now()))
	h.expect(h.do("POST", "/signatures", prodKey, `{"id":"n11"}`, "X-Organization-Id", org), 200)
}

func TestRateLimit(t *testing.T) {
	clk := &clock{now: time.Date(2026, 11, 20, 10, 0, 10, 0, time.UTC)}
	h := newHarness(t, func(d *Deps) { d.Now = clk.Now })
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "Proveedor")
	org := h.organization(ana, ws, "Empresa", "0614", "SV")
	h.certificate(ana, ws, org, "test", "0614")
	key := h.key(ana, ws, "test", []string{"sign", "keys:write"}, "")
	sign := func(k string) response {
		return h.do("POST", "/signatures", k, `{"id":"x"}`, "X-Organization-Id", org)
	}

	// Without limits, no rate headers.
	if r := h.expect(sign(key), 200); r.header.Get("X-RateLimit-Limit") != "" {
		t.Fatalf("headers without a limit: %v", r.header)
	}

	// The install's limit (here, the console's) for the space.
	setLimits(t, h.db, ws, store.LimitsPatch{RequestsPerMinute: limit(3)})
	clk.add(time.Minute)
	for i := range 3 {
		r := h.expect(sign(key), 200)
		if r.header.Get("X-RateLimit-Limit") != "3" || r.header.Get("X-RateLimit-Remaining") != fmt.Sprint(2-i) ||
			r.header.Get("X-RateLimit-Reset") != "50" {
			t.Fatalf("headers: %v", r.header)
		}
	}
	r := h.expect(sign(key), 429, "rate-limited")
	if p := problemOf(t, r); p.AppliesTo != "workspace" || *p.Limit != 3 || r.header.Get("Retry-After") != "50" {
		t.Fatalf("problem = %s, Retry-After %q", r.body, r.header.Get("Retry-After"))
	}
	// The next minute, again.
	clk.add(time.Minute)
	h.expect(sign(key), 200)

	// The space lowers it for itself, never above its ceiling.
	h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"requests_per_minute": 4}), 400, "invalid-request")
	h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"requests_per_minute": 0}), 400, "invalid-request")
	got := h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"requests_per_minute": 2}), 200).json(t)
	if got["requests_per_minute"] != float64(2) || got["limits"].(map[string]any)["requests_per_minute"] != float64(3) {
		t.Fatalf("workspace = %v", got)
	}
	clk.add(time.Minute)
	h.expect(sign(key), 200)
	h.expect(sign(key), 200)
	h.expect(sign(key), 429, "rate-limited")
	got = h.expect(h.do("PATCH", "/workspaces/"+ws, ana, map[string]any{"requests_per_minute": nil}), 200).json(t)
	if got["requests_per_minute"] != nil {
		t.Fatalf("cleared = %v", got)
	}

	// A key's own limit, on top of the space's.
	h.expect(h.do("POST", "/workspaces/"+ws+"/api-keys", ana,
		map[string]any{"name": "lenta", "environment": "test", "scopes": []string{"sign", "keys:write"}, "requests_per_minute": 0}), 400)
	created := h.expect(h.do("POST", "/workspaces/"+ws+"/api-keys", ana,
		map[string]any{"name": "lenta", "environment": "test", "scopes": []string{"sign", "keys:write"}, "requests_per_minute": 1}), 201).json(t)
	slow := str(created, "key")
	if created["requests_per_minute"] != float64(1) {
		t.Fatalf("key = %v", created)
	}
	clk.add(time.Minute)
	h.expect(sign(slow), 200)
	r = h.expect(sign(slow), 429, "rate-limited")
	if p := problemOf(t, r); p.AppliesTo != "api_key" || *p.Limit != 1 {
		t.Fatalf("problem = %s", r.body)
	}
	// The other key still signs.
	h.expect(sign(key), 200)
	// A key with a limit gives no more than it has.
	h.expect(h.do("POST", "/workspaces/self/api-keys", slow,
		map[string]any{"name": "x", "environment": "test", "scopes": []string{"sign"}}), 403, "forbidden")
	h.expect(h.do("POST", "/workspaces/self/api-keys", slow,
		map[string]any{"name": "x", "environment": "test", "scopes": []string{"sign"}, "requests_per_minute": 2}), 403, "forbidden")
	h.expect(h.do("POST", "/workspaces/self/api-keys", slow,
		map[string]any{"name": "x", "environment": "test", "scopes": []string{"sign"}, "requests_per_minute": 1}), 201)
	keys := h.expect(h.do("GET", "/workspaces/"+ws+"/api-keys", ana, nil), 200).items(t)
	if keys[len(keys)-1]["requests_per_minute"] != nil {
		t.Fatalf("the first key's limit = %v", keys[len(keys)-1])
	}
}

func TestInstallDefaults(t *testing.T) {
	h := newHarness(t, nil)
	rpm, days := 100, 30
	h.db.WithDefaults(store.Defaults{RequestsPerMinute: &rpm, RetentionDays: &days})
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "Proveedor")
	l := h.expect(h.do("GET", "/workspaces/"+ws, ana, nil), 200).json(t)["limits"].(map[string]any)
	if l["requests_per_minute"] != float64(100) || l["retention_days"] != float64(30) || l["prod_signatures"] != nil {
		t.Fatalf("limits = %v", l)
	}
	// The console's win over the install's.
	setLimits(t, h.db, ws, store.LimitsPatch{RetentionDays: limit(7)})
	l = h.expect(h.do("GET", "/workspaces/"+ws, ana, nil), 200).json(t)["limits"].(map[string]any)
	if l["retention_days"] != float64(7) {
		t.Fatalf("limits = %v", l)
	}
	// A space with a plan takes nothing of the install's, and the console
	// doesn't set its limits: billing pushes them.
	if _, err := h.db.Pool().Exec(context.Background(), `UPDATE workspaces SET plan = 'enterprise', retention_days = NULL,
		requests_per_minute = NULL WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	l = h.expect(h.do("GET", "/workspaces/"+ws, ana, nil), 200).json(t)["limits"].(map[string]any)
	if l["requests_per_minute"] != nil || l["retention_days"] != nil {
		t.Fatalf("enterprise = %v", l)
	}
	if _, err := h.db.SetLimits(context.Background(), ws, store.LimitsPatch{RetentionDays: limit(1)}); err != store.ErrHasPlan {
		t.Fatalf("SetLimits on a plan: %v", err)
	}
}

func TestOrganizationLimit(t *testing.T) {
	h := newHarness(t, nil)
	ana := h.person("ana", auth.Claims{})
	ws := h.space(ana, "Proveedor")
	setLimits(t, h.db, ws, store.LimitsPatch{Organizations: limit(1)})
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "Una", "tax_id": "0614-1", "country": "SV"}), 201)
	r := h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "Otra", "tax_id": "0614-2", "country": "SV"}), 409, "organization-limit-reached")
	if p := problemOf(t, r); p.Limit == nil || *p.Limit != 1 {
		t.Fatalf("problem = %s", r.body)
	}
	// Asking again for the one it has isn't another.
	h.expect(h.do("POST", "/workspaces/"+ws+"/organizations", ana,
		map[string]any{"name": "Una", "tax_id": "06141", "country": "SV"}), 200)

	// Bringing them from Sello's space counts too: all or none.
	ctx := context.Background()
	platform, err := h.db.CreateWorkspace(ctx, "Sello", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var claimed []string
	for i, sello := range []string{"idp-a", "idp-b"} {
		o, _, err := h.db.CreateOrganization(ctx, store.NewOrganization{
			WorkspaceID: platform.ID, Name: sello, TaxID: fmt.Sprint(i), Country: "SV", SelloWorkspaceID: &sello,
		})
		if err != nil {
			t.Fatal(err)
		}
		claimed = append(claimed, o.ID)
	}
	setLimits(t, h.db, ws, store.LimitsPatch{Organizations: limit(2)})
	if _, err := h.db.ClaimOrganizations(ctx, ws, platform.ID, claimed, []string{"idp-a", "idp-b"}); err != store.ErrOrganizationLimit {
		t.Fatalf("claiming two past the limit: %v", err)
	}
	if _, err := h.db.ClaimOrganizations(ctx, ws, platform.ID, claimed[:1], []string{"idp-a"}); err != nil {
		t.Fatalf("claiming one: %v", err)
	}
}

// logEntry writes an entry of the signing log signed at, with an id of
// that instant (RecordSignature's are the process's, never older than its
// last one).
func logEntry(t *testing.T, db *store.Store, signer, org string, at time.Time, n int) {
	t.Helper()
	floor := ids.Floor(at)
	id := floor[:14] + "7000-8000-" + fmt.Sprintf("%012x", n)
	if _, err := db.Pool().Exec(context.Background(), `INSERT INTO signing_log
		(id, organization_id, signed_by_workspace_id, environment, duration_ms, created_at)
		VALUES ($1, $2, $3, 'test', 1, $4)`, id, org, signer, at); err != nil {
		t.Fatal(err)
	}
}

func TestRetention(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	ana, carl := h.person("ana", auth.Claims{}), h.person("carl", auth.Claims{})
	short := h.space(ana, "Siete días")
	forever := h.space(carl, "Sin plazo")
	org := h.organization(ana, short, "Empresa", "0614", "SV")
	setLimits(t, h.db, short, store.LimitsPatch{RetentionDays: limit(7)})

	now := time.Now().UTC()
	old, recent := now.AddDate(0, 0, -8), now.AddDate(0, 0, -6)
	logEntry(t, h.db, short, org, old, 1)
	logEntry(t, h.db, short, org, old.Add(time.Hour), 2)
	logEntry(t, h.db, short, org, recent, 3)
	// What another space signed with the organization follows that space's
	// retention: none.
	logEntry(t, h.db, forever, org, old, 4)
	if _, err := h.db.Pool().Exec(ctx, `INSERT INTO usage_counters (workspace_id, organization_id, environment, month, count)
		VALUES ($1, $2, 'test', $3, 3)`, short, org, model.Month(old)); err != nil {
		t.Fatal(err)
	}

	// Another instance is purging: this one doesn't.
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _ = h.db.TryLock(ctx, 0x5e110_5169, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	if ran, _, err := retention.Once(ctx, h.db, now); ran || err != nil {
		t.Fatalf("ran beside another: %v %v", ran, err)
	}
	close(release)

	var ran bool
	var deleted int64
	var err error
	for range 50 {
		if ran, deleted, err = retention.Once(ctx, h.db, now); ran || err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ran || err != nil || deleted != 2 {
		t.Fatalf("purge: ran %v deleted %d %v", ran, deleted, err)
	}
	log := h.expect(h.do("GET", "/workspaces/"+short+"/signatures", ana, nil), 200).items(t)
	if len(log) != 2 {
		// Its recent one, and the other space's, which it owns the
		// organization of.
		t.Fatalf("the log kept %d", len(log))
	}
	// What was counted stays: it's billed.
	if items := h.expect(h.do("GET", "/workspaces/"+short+"/usage?from=2000-01", ana, nil), 200).json(t)["items"].([]any); len(items) != 1 {
		t.Fatalf("usage = %v", items)
	}
	// Again, nothing left to delete.
	if _, deleted, _ := retention.Once(ctx, h.db, now); deleted != 0 {
		t.Fatalf("deleted again %d", deleted)
	}
}
