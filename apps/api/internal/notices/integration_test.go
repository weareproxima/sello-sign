package notices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// testStore creates a database of its own in POSTGRES_TEST_URL's server,
// migrates it and drops it when the test ends. Without the variable, the
// test is skipped.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		t.Skip("POSTGRES_TEST_URL is unset")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "sign_test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	if _, err := store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return store.New(pool)
}

// pg writes the rows of a test straight to the database.
type pg struct {
	t  *testing.T
	db *store.Store
}

func (p pg) exec(sql string, args ...any) {
	p.t.Helper()
	if _, err := p.db.Pool().Exec(context.Background(), sql, args...); err != nil {
		p.t.Fatalf("%s: %v", sql, err)
	}
}

func (p pg) count(sql string, args ...any) int {
	p.t.Helper()
	var n int
	if err := p.db.Pool().QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		p.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (p pg) user(id, email, selloSub string) {
	var sub any
	if selloSub != "" {
		sub = selloSub
	}
	p.exec(`INSERT INTO users (id, email, sello_sub) VALUES ($1, $2, $3)`, id, email, sub)
}

func (p pg) space(name string, members map[string]model.Role) string {
	id := ids.New()
	p.exec(`INSERT INTO workspaces (id, name) VALUES ($1, $2)`, id, name)
	for u, r := range members {
		p.exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)`, id, u, string(r))
	}
	return id
}

func (p pg) organization(ws, name string) string {
	id := ids.New()
	p.exec(`INSERT INTO organizations (id, workspace_id, name, tax_id, tax_id_key, country) VALUES ($1, $2, $3, $4, $4, 'SV')`,
		id, ws, name, id[len(id)-12:])
	return id
}

func (p pg) certificate(org string, env model.Environment, notAfter time.Time, active bool) string {
	id := ids.New()
	p.exec(`INSERT INTO certificates (id, organization_id, environment, material, not_after, active)
		VALUES ($1, $2, $3, '\x00', $4, $5)`, id, org, string(env), notAfter, active)
	return id
}

func (p pg) signed(ws, org string, env model.Environment, month string, n int) {
	p.exec(`INSERT INTO usage_counters (workspace_id, organization_id, environment, month, count) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (workspace_id, organization_id, environment, month) DO UPDATE SET count = usage_counters.count + EXCLUDED.count`,
		ws, org, string(env), month, n)
}

// sentTo is who got each subject, sorted.
func sentTo(m *fakeMail) []string {
	out := m.to()
	sort.Strings(out)
	return out
}

func TestNoticesAgainstPostgres(t *testing.T) {
	db := testStore(t)
	p := pg{t, db}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	day := 24 * time.Hour
	month := model.Month(now)

	// A space of an install: its owner and admins are told; a member, an
	// admin who never signed in and one without an address aren't.
	p.user("ana", "ana@example.com", "")
	p.user("bob", "Bob@Example.com", "")
	p.user("carl", "carl@example.com", "")
	p.user("erin", "", "")
	provider := p.space("Proveedor", map[string]model.Role{"ana": model.Owner, "bob": model.Admin, "carl": model.Member, "dana": model.Admin, "erin": model.Admin})
	o1 := p.organization(provider, "Empresa Uno")
	c1 := p.certificate(o1, model.Prod, now.Add(5*day), true) // 7 days
	p.certificate(o1, model.Test, now.Add(40*day), true)      // not yet
	p.certificate(o1, model.Prod, now.Add(time.Hour), false)  // inactive
	p.certificate(o1, model.Test, now.Add(-400*day), false)   // inactive, expired
	o2 := p.organization(provider, "Empresa Dos")
	c2 := p.certificate(o2, model.Test, now.Add(-2*day), true) // expired, still active

	// A space that signs with o1 by delegation isn't told.
	p.user("dave", "dave@example.com", "")
	delegate := p.space("Delegado", map[string]model.Role{"dave": model.Owner})
	p.exec(`INSERT INTO delegations (id, organization_id, workspace_id, scopes) VALUES ($1, $2, $3, '{sign,certificates:write}')`,
		ids.New(), o1, delegate)

	// Sello's platform space and a closed one aren't either.
	p.user("staff", "staff@example.com", "")
	platform := p.space("Sello", map[string]model.Role{"staff": model.Owner})
	p.certificate(p.organization(platform, "Contribuyente"), model.Prod, now.Add(day/2), true)
	p.user("gone", "gone@example.com", "")
	closed := p.space("Cerrado", map[string]model.Role{"gone": model.Owner})
	p.exec(`UPDATE workspaces SET billing_closed_at = now() WHERE id = $1`, closed)
	p.certificate(p.organization(closed, "Cerrada"), model.Prod, now.Add(day/2), true)

	// A space of the cloud: its seats, as far as its plan admits, by their
	// Sello account; the same address twice is one.
	p.user("owner-panel", "owner@cloud.example", "acct-1")
	p.user("seat2-panel", "seat2@cloud.example", "acct-2")
	p.user("also-owner", "OWNER@cloud.example", "")
	cloud := p.space("Nube", map[string]model.Role{"also-owner": model.Admin})
	p.exec(`UPDATE workspaces SET sello_workspace_id = 'sello-1', plan = 'free', max_members = 1,
			max_prod_signatures = 500, prod_hard_cap = 500, max_test_signatures = 5000, test_hard_cap = 5000 WHERE id = $1`, cloud)
	p.exec(`INSERT INTO billing_members (workspace_id, account, role, position) VALUES ($1, 'acct-1', 'owner', 0), ($1, 'acct-2', 'admin', 1)`, cloud)
	o3 := p.organization(cloud, "Empresa Tres")
	c3 := p.certificate(o3, model.Prod, now.Add(10*day), true) // 30 days
	p.signed(cloud, o3, model.Prod, month, 450)                // 90 % of 500
	p.signed(cloud, o3, model.Test, month, 10)
	// Without limits, no usage notice.
	p.signed(provider, o1, model.Prod, month, 100_000)
	// The usage notices of long ago go.
	p.exec(`INSERT INTO usage_notices (workspace_id, environment, month, threshold, limit_value) VALUES ($1, 'prod', '2000-01', 'cap', 1)`, cloud)

	if got, err := db.NoticeRecipients(ctx, cloud); err != nil || len(got) != 1 || !strings.EqualFold(got[0], "owner@cloud.example") {
		t.Fatalf("cloud recipients: %q %v", got, err)
	}

	m := &fakeMail{}
	n := &Notifier{Source: db, Mail: m, PanelURL: "https://sign.sello.tax", Platform: platform}
	ran, res, err := n.Once(ctx, now)
	if !ran || err != nil || res != (Result{Sent: 4}) {
		t.Fatalf("once: %v %+v %v", ran, res, err)
	}
	cloudTo := m.sent[len(m.sent)-1].To // whichever of the two addresses came first
	want := []string{
		"Bob@Example.com: Certificados vencidos y por vencer en el espacio «Proveedor»",
		"ana@example.com: Certificados vencidos y por vencer en el espacio «Proveedor»",
		cloudTo + ": El certificado de producción de «Empresa Tres» vence el " + formatDate(now.Add(10*day)),
		cloudTo + ": El espacio «Nube» usó el 80 % de sus firmas de producción del mes",
	}
	sort.Strings(want)
	if got := sentTo(m); !slices.Equal(got, want) {
		t.Fatalf("sent:\n%q\nwant:\n%q", got, want)
	}
	for _, msg := range m.sent {
		if strings.Contains(msg.Subject, "Proveedor") && !strings.Contains(msg.Text, "https://sign.sello.tax/w/"+provider+"/organizations/"+o2) {
			t.Fatalf("no link to the organization:\n%s", msg.Text)
		}
	}
	if p.count(`SELECT count(*) FROM certificate_notices`) != 3 ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1 AND threshold = '7d'`, c1) != 1 ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1 AND threshold = 'expired'`, c2) != 1 ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1 AND threshold = '30d'`, c3) != 1 {
		t.Fatal("certificate notices not recorded")
	}
	if p.count(`SELECT count(*) FROM usage_notices`) != 1 ||
		p.count(`SELECT count(*) FROM usage_notices WHERE workspace_id = $1 AND environment = 'prod' AND month = $2
			AND threshold = 'quota-80' AND limit_value = 500`, cloud, month) != 1 {
		t.Fatal("usage notice not recorded, or the old one not purged")
	}

	// Again, or on another instance after it: nothing.
	m.sent = nil
	if _, res, err := n.Once(ctx, now.Add(time.Hour)); err != nil || res != (Result{}) || len(m.sent) != 0 {
		t.Fatalf("again: %+v %v %q", res, err, m.to())
	}

	// The cap, once reached.
	p.signed(cloud, o3, model.Prod, month, 50)
	if _, res, _ := n.Once(ctx, now.Add(2*time.Hour)); res != (Result{Sent: 1}) ||
		!slices.Equal(m.to(), []string{cloudTo + ": El espacio «Nube» alcanzó el límite de firmas de producción del mes"}) {
		t.Fatalf("cap: %+v %q", res, m.to())
	}

	// A day before c1 expires, its last notice (and c3's of 7 days); the
	// mail service fails for one of c1's: it waits for the next run.
	m.sent = nil
	m.fail = map[string]error{"ana@example.com": context.DeadlineExceeded}
	at := now.Add(4*day + time.Hour)
	if _, res, _ := n.Once(ctx, at); res != (Result{Sent: 1, Pending: 1}) ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1`, c1) != 1 ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1 AND threshold = '7d'`, c3) != 1 {
		t.Fatalf("down: %+v", res)
	}
	m.fail = nil
	if _, res, _ := n.Once(ctx, at.Add(time.Hour)); res != (Result{Sent: 1}) ||
		p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1 AND threshold = '1d'`, c1) != 1 {
		t.Fatalf("back: %+v", res)
	}

	// Another instance is sending them: this one doesn't.
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = db.TryLock(ctx, lock, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	if ran, _, err := n.Once(ctx, at.Add(2*time.Hour)); ran || err != nil {
		t.Fatalf("ran beside another: %v %v", ran, err)
	}
	close(release)

	// The rows go with their certificate, and one deleted meanwhile isn't
	// recorded.
	p.exec(`DELETE FROM certificates WHERE id = $1`, c1)
	if p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1`, c1) != 0 {
		t.Fatal("the notices outlived their certificate")
	}
	if err := db.MarkCertificateNotices(ctx, []store.CertificateNotice{{CertificateID: c1, Threshold: Expired}}); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkUsageNotice(ctx, store.UsageNotice{WorkspaceID: ids.New(), Environment: model.Prod, Month: month, Threshold: CapReached, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if p.count(`SELECT count(*) FROM certificate_notices WHERE certificate_id = $1`, c1) != 0 || p.count(`SELECT count(*) FROM usage_notices`) != 2 {
		t.Fatal("recorded a notice of something gone")
	}
}
