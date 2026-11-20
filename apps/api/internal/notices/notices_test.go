package notices

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/mail"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

func intp(n int) *int { return &n }

func TestCertificateThreshold(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, c := range []struct {
		left time.Duration
		want string
	}{
		{31 * day, ""},
		{30*day + time.Second, ""},
		{30 * day, Days30},
		{8 * day, Days30},
		{7 * day, Days7},
		{day + time.Second, Days7},
		{day, Days1},
		{time.Second, Days1},
		{0, Expired},
		{-400 * day, Expired},
	} {
		got, ok := CertificateThreshold(now.Add(c.left), now)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%v left: %q %v, want %q", c.left, got, ok, c.want)
		}
	}
}

func TestCertificateDue(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	in5 := now.Add(5 * 24 * time.Hour)
	for _, c := range []struct {
		notAfter time.Time
		sent     []string
		want     string
	}{
		// Uploaded 5 days before it expires: the 7 days' notice alone.
		{in5, nil, Days7},
		{in5, []string{Days30}, Days7},
		{in5, []string{Days30, Days7}, ""},
		// Never one less pressing after a more pressing one.
		{in5, []string{Days1}, ""},
		{now.Add(-time.Hour), []string{Days30, Days7, Days1}, Expired},
		{now.Add(-time.Hour), []string{Expired}, ""},
		{now.Add(40 * 24 * time.Hour), nil, ""},
	} {
		got, ok := certificateDue(c.notAfter, now, c.sent)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%v %v: %q %v, want %q", c.notAfter, c.sent, got, ok, c.want)
		}
	}
}

func TestUsageThreshold(t *testing.T) {
	for _, c := range []struct {
		name       string
		used       int64
		quota, cap *int
		want       string
		limit      int
	}{
		{"nothing signed", 0, intp(500), intp(500), "", 0},
		{"no limits", 10_000, nil, nil, "", 0},
		{"free, below 80 %", 399, intp(500), intp(500), "", 0},
		{"free, 80 %", 400, intp(500), intp(500), QuotaWarning, 500},
		{"free, at its quota, which is its cap", 500, intp(500), intp(500), CapReached, 500},
		{"free, past it", 503, intp(500), intp(500), CapReached, 500},
		{"a quota without a cap (the console's)", 450, intp(500), nil, QuotaWarning, 500},
		{"the console's, reached", 500, intp(500), nil, CapReached, 500},
		{"a cap without a quota", 100, nil, intp(100), CapReached, 100},
		{"paid prod, 80 %", 4_000, intp(5_000), intp(10_000), QuotaWarning, 5_000},
		{"paid prod, its quota: the overage starts", 5_000, intp(5_000), intp(10_000), QuotaReached, 5_000},
		{"paid prod, in the overage", 9_999, intp(5_000), intp(10_000), QuotaReached, 5_000},
		{"paid prod, its cap", 10_000, intp(5_000), intp(10_000), CapReached, 10_000},
		{"paid test, its margin", 50_100, intp(50_000), intp(55_000), QuotaReached, 50_000},
		{"a quota of zero", 1, intp(0), intp(0), CapReached, 0},
		{"a small quota", 1, intp(2), intp(2), "", 0},
	} {
		got, limit, ok := UsageThreshold(c.used, c.quota, c.cap)
		if got != c.want || limit != c.limit || ok != (c.want != "") {
			t.Errorf("%s: %q %d %v, want %q %d", c.name, got, limit, ok, c.want, c.limit)
		}
	}
}

func TestFormats(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 55_000: "55,000", 1_234_567: "1,234,567", -1500: "-1,500"} {
		if got := formatNumber(n); got != want {
			t.Errorf("formatNumber(%d) = %q, want %q", n, got, want)
		}
	}
	at := time.Date(2026, 12, 1, 3, 5, 0, 0, time.FixedZone("CST", -6*3600))
	if got := formatDateTime(at); got != "1 de diciembre de 2026 a las 09:05 UTC" {
		t.Errorf("formatDateTime = %q", got)
	}
	if got := formatDate(nextMonth("2026-12")); got != "1 de enero de 2027" {
		t.Errorf("nextMonth = %q", got)
	}
}

const panel = "https://sign.sello.tax"

func cert(id, ws, org string, env model.Environment, notAfter time.Time) store.ExpiringCertificate {
	return store.ExpiringCertificate{
		ID: id, Environment: env, NotAfter: notAfter,
		OrganizationID: org, OrganizationName: "Empresa " + org, WorkspaceID: ws, WorkspaceName: "Espacio " + ws,
	}
}

func TestCertificateMessages(t *testing.T) {
	at := time.Date(2026, 12, 20, 14, 5, 0, 0, time.UTC)
	c := cert("c1", "w1", "o1", model.Prod, at)

	m := certificateMessage(panel, []certificateItem{{c, Days30}})
	if m.Subject != "El certificado de producción de «Empresa o1» vence el 20 de diciembre de 2026" {
		t.Errorf("subject %q", m.Subject)
	}
	wantText := "El certificado de producción de «Empresa o1», en el espacio «Espacio w1» de Sello Sign, vence el 20 de diciembre de 2026 a las 14:05 UTC. Desde entonces, Sello Sign no firmará con él.\n\n" +
		"Para seguir firmando, suba el certificado nuevo antes de esa fecha:\nhttps://sign.sello.tax/w/w1/organizations/o1\n\n" +
		"Recibe este aviso porque administra el espacio «Espacio w1» en Sello Sign.\n"
	if m.Text != wantText {
		t.Errorf("text:\n%s\nwant:\n%s", m.Text, wantText)
	}
	if !strings.Contains(m.HTML, `<a href="https://sign.sello.tax/w/w1/organizations/o1">`) {
		t.Errorf("html %s", m.HTML)
	}

	c.Environment = model.Test
	m = certificateMessage(panel, []certificateItem{{c, Expired}})
	if m.Subject != "El certificado de prueba de «Empresa o1» venció" ||
		!strings.HasPrefix(m.Text, "El certificado de prueba de «Empresa o1», en el espacio «Espacio w1» de Sello Sign, venció el 20 de diciembre de 2026 a las 14:05 UTC y sigue activo: Sello Sign ya no firma con él.\n\nPara volver a firmar, suba un certificado nuevo:\n") {
		t.Errorf("expired: %q\n%s", m.Subject, m.Text)
	}

	// Several in one e-mail, in the order given.
	soon := cert("c2", "w1", "o2", model.Prod, at)
	gone := cert("c3", "w1", "o3", model.Test, at.AddDate(0, -1, 0))
	m = certificateMessage(panel, []certificateItem{{gone, Expired}, {soon, Days7}})
	if m.Subject != "Certificados vencidos y por vencer en el espacio «Espacio w1»" {
		t.Errorf("subject %q", m.Subject)
	}
	for _, want := range []string{
		"Estos certificados del espacio «Espacio w1» de Sello Sign vencen pronto o ya vencieron.",
		"- «Empresa o3», prueba: venció el 20 de noviembre de 2026 a las 14:05 UTC.\n  https://sign.sello.tax/w/w1/organizations/o3\n" +
			"- «Empresa o2», producción: vence el 20 de diciembre de 2026 a las 14:05 UTC.\n  https://sign.sello.tax/w/w1/organizations/o2\n",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, m.Text)
		}
	}
	if m = certificateMessage(panel, []certificateItem{{soon, Days7}, {soon, Days30}}); m.Subject != "Certificados por vencer en el espacio «Espacio w1»" {
		t.Errorf("subject %q", m.Subject)
	}
	if m = certificateMessage(panel, []certificateItem{{gone, Expired}, {gone, Expired}}); m.Subject != "Certificados vencidos en el espacio «Espacio w1»" ||
		!strings.Contains(m.Text, "vencieron y siguen activos: Sello Sign ya no firma con ellos.") {
		t.Errorf("all expired: %q\n%s", m.Subject, m.Text)
	}

	// Names are escaped in the HTML, never in the text.
	odd := cert("c4", "w1", "o4", model.Prod, at)
	odd.OrganizationName = `<b>"A & B"</b>`
	m = certificateMessage(panel, []certificateItem{{odd, Days30}})
	if strings.Contains(m.HTML, "<b>") || !strings.Contains(m.HTML, "&lt;b&gt;&#34;A &amp; B&#34;&lt;/b&gt;") ||
		!strings.Contains(m.Text, `«<b>"A & B"</b>»`) {
		t.Errorf("escaping:\n%s\n%s", m.HTML, m.Text)
	}
}

func TestUsageMessages(t *testing.T) {
	free := "free"
	u := store.SpaceUsage{WorkspaceID: "w1", Name: "Pruebas", Plan: &free, Environment: model.Prod, Quota: intp(500), Cap: intp(500)}
	m := usageMessage(panel, u, QuotaWarning, "2026-11")
	if m.Subject != "El espacio «Pruebas» usó el 80 % de sus firmas de producción del mes" {
		t.Errorf("subject %q", m.Subject)
	}
	want := "El espacio «Pruebas» de Sello Sign usó el 80 % de las 500 firmas de producción de este mes que incluye su plan. Al llegar a 500, Sello Sign rechazará las siguientes hasta el 1 de diciembre de 2026, cuando la cuenta empieza de nuevo.\n\n" +
		"Vea el uso del espacio en Sello Sign:\nhttps://sign.sello.tax/w/w1\n\n" +
		"Recibe este aviso porque administra el espacio «Pruebas» en Sello Sign.\n"
	if m.Text != want {
		t.Errorf("text:\n%s\nwant:\n%s", m.Text, want)
	}
	m = usageMessage(panel, u, CapReached, "2026-12")
	if m.Subject != "El espacio «Pruebas» alcanzó el límite de firmas de producción del mes" ||
		!strings.HasPrefix(m.Text, "Alcanzó el límite de 500 firmas de producción de este mes en el espacio «Pruebas» de Sello Sign. Sello Sign rechaza las siguientes hasta el 1 de enero de 2027, cuando la cuenta empieza de nuevo. Para firmar más este mes, puede cambiar el plan del espacio.\n") {
		t.Errorf("cap: %q\n%s", m.Subject, m.Text)
	}

	growth := "pro_growth"
	paid := store.SpaceUsage{WorkspaceID: "w2", Name: "Proveedor", Plan: &growth, Environment: model.Prod, Quota: intp(5_000), Cap: intp(10_000)}
	if m = usageMessage(panel, paid, QuotaWarning, "2026-11"); !strings.Contains(m.Text,
		"usó el 80 % de las 5,000 firmas de producción de este mes que incluye su plan. Después de 5,000, cada firma de producción se cobra como excedente, hasta un tope de 10,000.\n") {
		t.Errorf("paid warning:\n%s", m.Text)
	}
	m = usageMessage(panel, paid, QuotaReached, "2026-11")
	if m.Subject != "El espacio «Proveedor» usó todas las firmas de producción incluidas este mes" || !strings.HasPrefix(m.Text,
		"El espacio «Proveedor» de Sello Sign llegó a las 5,000 firmas de producción de este mes que incluye su plan. Las siguientes se cobran como excedente, hasta un tope de 10,000; después, Sello Sign las rechazará hasta el 1 de diciembre de 2026.\n") {
		t.Errorf("paid quota: %q\n%s", m.Subject, m.Text)
	}
	paid.Environment, paid.Quota, paid.Cap = model.Test, intp(50_000), intp(55_000)
	if m = usageMessage(panel, paid, QuotaWarning, "2026-11"); !strings.Contains(m.Text,
		"de las 50,000 firmas de prueba de este mes que incluye su plan. Después de 50,000, puede firmar hasta 55,000; luego, Sello Sign rechazará las siguientes hasta el 1 de diciembre de 2026.\n") {
		t.Errorf("test warning:\n%s", m.Text)
	}
	if m = usageMessage(panel, paid, QuotaReached, "2026-11"); !strings.Contains(m.Text,
		"llegó a las 50,000 firmas de prueba de este mes que incluye su plan. Puede firmar hasta 55,000; después, Sello Sign rechazará las siguientes hasta el 1 de diciembre de 2026.\n") {
		t.Errorf("test quota:\n%s", m.Text)
	}

	// Without a plan, the install's limit, and nothing about changing a plan.
	own := store.SpaceUsage{WorkspaceID: "w3", Name: "Propio", Environment: model.Prod, Quota: intp(1_000)}
	if m = usageMessage(panel, own, QuotaWarning, "2026-11"); !strings.Contains(m.Text, "de las 1,000 firmas de producción de este mes que le permite la instalación. Al llegar a 1,000,") {
		t.Errorf("own warning:\n%s", m.Text)
	}
	if m = usageMessage(panel, own, CapReached, "2026-11"); strings.Contains(m.Text, "plan") {
		t.Errorf("own cap:\n%s", m.Text)
	}
}

// fakeSource is the database in memory.
type fakeSource struct {
	locked     bool
	certs      []store.ExpiringCertificate
	certSent   map[string][]string
	usage      []store.SpaceUsage
	usageSent  map[store.UsageNotice]bool
	recipients map[string][]string
	purged     []string
}

func newFakeSource() *fakeSource {
	return &fakeSource{certSent: map[string][]string{}, usageSent: map[store.UsageNotice]bool{}, recipients: map[string][]string{}}
}

func (f *fakeSource) TryLock(ctx context.Context, _ int64, fn func(context.Context) error) (bool, error) {
	if f.locked {
		return false, nil
	}
	return true, fn(ctx)
}

func (f *fakeSource) ExpiringCertificates(_ context.Context, before time.Time, except string) ([]store.ExpiringCertificate, error) {
	var out []store.ExpiringCertificate
	for _, c := range f.certs {
		if !c.NotAfter.After(before) && c.WorkspaceID != except {
			c.Sent = slices.Clone(f.certSent[c.ID])
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeSource) MarkCertificateNotices(_ context.Context, sent []store.CertificateNotice) error {
	for _, n := range sent {
		f.certSent[n.CertificateID] = append(f.certSent[n.CertificateID], n.Threshold)
	}
	return nil
}

func (f *fakeSource) NoticeUsage(_ context.Context, _, except string) ([]store.SpaceUsage, error) {
	var out []store.SpaceUsage
	for _, u := range f.usage {
		if u.WorkspaceID != except {
			out = append(out, u)
		}
	}
	return out, nil
}

func (f *fakeSource) UsageNoticesSent(_ context.Context, month string) ([]store.UsageNotice, error) {
	var out []store.UsageNotice
	for n := range f.usageSent {
		if n.Month == month {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeSource) MarkUsageNotice(_ context.Context, n store.UsageNotice) error {
	f.usageSent[n] = true
	return nil
}

func (f *fakeSource) PurgeUsageNotices(_ context.Context, before string) (int64, error) {
	f.purged = append(f.purged, before)
	return 0, nil
}

func (f *fakeSource) NoticeRecipients(_ context.Context, ws string) ([]string, error) {
	return f.recipients[ws], nil
}

// fakeMail takes every message, except for the addresses in fail, which
// get that error.
type fakeMail struct {
	sent []mail.Message
	fail map[string]error
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	if err := f.fail[m.To]; err != nil {
		return err
	}
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) to() []string {
	var out []string
	for _, m := range f.sent {
		out = append(out, m.To+": "+m.Subject)
	}
	return out
}

func TestOnceCertificates(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	src := newFakeSource()
	src.certs = []store.ExpiringCertificate{
		cert("c1", "w1", "o1", model.Prod, now.Add(5*day)),
		cert("c2", "w1", "o2", model.Test, now.Add(20*day)),
		cert("c3", "w2", "o3", model.Prod, now.Add(-day)),
		cert("c4", "w2", "o4", model.Prod, now.Add(60*day)),
		cert("c5", "platform", "o5", model.Prod, now.Add(day)),
	}
	src.recipients["w1"] = []string{"ana@example.com", "bob@example.com"}
	src.recipients["w2"] = []string{"carl@example.com"}
	src.recipients["platform"] = []string{"staff@example.com"}
	m := &fakeMail{}
	n := &Notifier{Source: src, Mail: m, PanelURL: panel, Platform: "platform"}

	ran, res, err := n.Once(context.Background(), now)
	if !ran || err != nil || res != (Result{Sent: 3}) {
		t.Fatalf("once: %v %+v %v", ran, res, err)
	}
	want := []string{
		"ana@example.com: Certificados por vencer en el espacio «Espacio w1»",
		"bob@example.com: Certificados por vencer en el espacio «Espacio w1»",
		"carl@example.com: El certificado de producción de «Empresa o3» venció",
	}
	if got := m.to(); !slices.Equal(got, want) {
		t.Fatalf("sent %q", got)
	}
	if !slices.Equal(src.certSent["c1"], []string{Days7}) || !slices.Equal(src.certSent["c2"], []string{Days30}) ||
		!slices.Equal(src.certSent["c3"], []string{Expired}) || len(src.certSent["c4"]) != 0 || len(src.certSent["c5"]) != 0 {
		t.Fatalf("recorded %v", src.certSent)
	}
	// Each recipient has a key of their own, the same in every run.
	if m.sent[0].IdempotencyKey == m.sent[1].IdempotencyKey || !strings.HasPrefix(m.sent[0].IdempotencyKey, "sign-notice-") {
		t.Fatalf("keys %q %q", m.sent[0].IdempotencyKey, m.sent[1].IdempotencyKey)
	}

	// Again: nothing new.
	m.sent = nil
	if _, res, _ := n.Once(context.Background(), now.Add(time.Hour)); res != (Result{}) || len(m.sent) != 0 {
		t.Fatalf("again: %+v %q", res, m.to())
	}
	// A day before c1 expires, its last notice, alone.
	if _, res, _ := n.Once(context.Background(), now.Add(4*day+time.Hour)); res != (Result{Sent: 1}) ||
		!slices.Equal(m.to(), []string{
			"ana@example.com: El certificado de producción de «Empresa o1» vence el 25 de noviembre de 2026",
			"bob@example.com: El certificado de producción de «Empresa o1» vence el 25 de noviembre de 2026",
		}) {
		t.Fatalf("1 day: %+v %q", res, m.to())
	}
}

func TestOnceRetries(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	src := newFakeSource()
	src.certs = []store.ExpiringCertificate{cert("c1", "w1", "o1", model.Prod, now.Add(48*time.Hour))}
	src.recipients["w1"] = []string{"ana@example.com", "bob@example.com"}
	m := &fakeMail{fail: map[string]error{"bob@example.com": errors.New("resend answered 503")}}
	n := &Notifier{Source: src, Mail: m, PanelURL: panel}

	// Bob's failed: it isn't recorded, and it's tried again.
	if _, res, err := n.Once(context.Background(), now); err != nil || res != (Result{Pending: 1}) || len(src.certSent["c1"]) != 0 {
		t.Fatalf("first: %+v %v %v", res, err, src.certSent)
	}
	first := m.sent[0]
	m.fail = nil
	if _, res, _ := n.Once(context.Background(), now.Add(time.Hour)); res != (Result{Sent: 1}) || !slices.Equal(src.certSent["c1"], []string{Days7}) {
		t.Fatalf("retry: %+v %v", res, src.certSent)
	}
	// Ana's is the same message with the same key: the mail service sends
	// it once.
	if again := m.sent[1]; again.To != first.To || again.IdempotencyKey != first.IdempotencyKey || again.Text != first.Text || again.HTML != first.HTML || again.Subject != first.Subject {
		t.Fatalf("retried as another message: %+v\n%+v", again, first)
	}

	// An address refused for good doesn't hold the others' notice…
	src.certs = append(src.certs, cert("c2", "w2", "o2", model.Prod, now.Add(48*time.Hour)))
	src.recipients["w2"] = []string{"bad@example.com", "carl@example.com"}
	m.fail = map[string]error{"bad@example.com": fmt.Errorf("%w: resend answered 422", mail.ErrRejected)}
	if _, res, _ := n.Once(context.Background(), now.Add(2*time.Hour)); res != (Result{Sent: 1}) || len(src.certSent["c2"]) != 1 {
		t.Fatalf("one refused: %+v %v", res, src.certSent)
	}
	// …but if nobody took it, it waits.
	src.certs = append(src.certs, cert("c3", "w3", "o3", model.Prod, now.Add(48*time.Hour)))
	src.recipients["w3"] = []string{"bad@example.com"}
	if _, res, _ := n.Once(context.Background(), now.Add(3*time.Hour)); res != (Result{Pending: 1}) || len(src.certSent["c3"]) != 0 {
		t.Fatalf("all refused: %+v %v", res, src.certSent)
	}
	// Nobody to tell: it waits too.
	src.recipients["w3"] = nil
	if _, res, _ := n.Once(context.Background(), now.Add(4*time.Hour)); res != (Result{Pending: 1}) {
		t.Fatalf("nobody: %+v", res)
	}
}

func TestOnceUsage(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	free, growth := "free", "pro_growth"
	src := newFakeSource()
	src.usage = []store.SpaceUsage{
		{WorkspaceID: "w1", Name: "Uno", Plan: &free, Environment: model.Prod, Used: 420, Quota: intp(500), Cap: intp(500)},
		{WorkspaceID: "w1", Name: "Uno", Plan: &free, Environment: model.Test, Used: 10, Quota: intp(5_000), Cap: intp(5_000)},
		{WorkspaceID: "w2", Name: "Dos", Plan: &growth, Environment: model.Prod, Used: 5_200, Quota: intp(5_000), Cap: intp(10_000)},
	}
	src.recipients["w1"] = []string{"ana@example.com"}
	src.recipients["w2"] = []string{"bob@example.com"}
	m := &fakeMail{}
	n := &Notifier{Source: src, Mail: m, PanelURL: panel}

	if _, res, err := n.Once(context.Background(), now); err != nil || res != (Result{Sent: 2}) {
		t.Fatalf("once: %+v %v", res, err)
	}
	if !slices.Equal(m.to(), []string{
		"ana@example.com: El espacio «Uno» usó el 80 % de sus firmas de producción del mes",
		"bob@example.com: El espacio «Dos» usó todas las firmas de producción incluidas este mes",
	}) {
		t.Fatalf("sent %q", m.to())
	}
	if !src.usageSent[store.UsageNotice{WorkspaceID: "w1", Environment: model.Prod, Month: "2026-11", Threshold: QuotaWarning, Limit: 500}] ||
		!src.usageSent[store.UsageNotice{WorkspaceID: "w2", Environment: model.Prod, Month: "2026-11", Threshold: QuotaReached, Limit: 5_000}] {
		t.Fatalf("recorded %v", src.usageSent)
	}
	// The usage notices of the months before October go.
	if !slices.Equal(src.purged, []string{"2026-10"}) {
		t.Fatalf("purged %v", src.purged)
	}

	// Again, nothing; at the cap, its notice.
	m.sent = nil
	if _, res, _ := n.Once(context.Background(), now.Add(time.Hour)); res != (Result{}) {
		t.Fatalf("again: %+v", res)
	}
	src.usage[0].Used = 500
	if _, res, _ := n.Once(context.Background(), now.Add(2*time.Hour)); res != (Result{Sent: 1}) ||
		!slices.Equal(m.to(), []string{"ana@example.com: El espacio «Uno» alcanzó el límite de firmas de producción del mes"}) {
		t.Fatalf("cap: %+v %q", res, m.to())
	}
	// Next month, the same count is another month's notice.
	m.sent = nil
	if _, res, _ := n.Once(context.Background(), time.Date(2026, 12, 1, 0, 30, 0, 0, time.UTC)); res != (Result{Sent: 2}) {
		t.Fatalf("next month: %+v %q", res, m.to())
	}
	// A plan that changes in the month is measured again.
	src.usage[0].Plan, src.usage[0].Quota, src.usage[0].Cap = &growth, intp(600), intp(1_200)
	m.sent = nil
	if _, res, _ := n.Once(context.Background(), time.Date(2026, 12, 1, 1, 30, 0, 0, time.UTC)); res != (Result{Sent: 1}) ||
		!slices.Equal(m.to(), []string{"ana@example.com: El espacio «Uno» usó el 80 % de sus firmas de producción del mes"}) {
		t.Fatalf("upgraded: %+v %q", res, m.to())
	}
}

func TestOnceLocked(t *testing.T) {
	src := newFakeSource()
	src.locked = true
	src.certs = []store.ExpiringCertificate{cert("c1", "w1", "o1", model.Prod, time.Now())}
	src.recipients["w1"] = []string{"ana@example.com"}
	m := &fakeMail{}
	if ran, _, err := (&Notifier{Source: src, Mail: m}).Once(context.Background(), time.Now()); ran || err != nil || len(m.sent) != 0 {
		t.Fatalf("ran beside another: %v %v %d", ran, err, len(m.sent))
	}
}
