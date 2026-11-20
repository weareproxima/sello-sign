package billing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/model"
)

const stateBody = `{
	"workspace_id": "org-1", "version": 7, "closed": false, "name": "Empresa Oc", "country": "sv",
	"members": [{"account": "sub-1", "role": "OWNER"}, {"account": "sub-2", "role": "MEMBER"}],
	"plan": "PRO_GROWTH",
	"limits": {"issuers": 10, "prod_signatures": 5000, "test_signatures": null, "members": 3, "requests_per_minute": 600, "retention_days": 90},
	"hard_caps": {"prod": 50000, "test": null},
	"subscription": {"status": "ACTIVE", "interval": "MONTH", "current_period_end": "2026-12-01T00:00:00Z", "cancel_at_period_end": false}
}`

func TestParseState(t *testing.T) {
	st, err := ParseState([]byte(stateBody))
	if err != nil {
		t.Fatal(err)
	}
	if st.WorkspaceID != "org-1" || st.Version != 7 || st.Plan != "pro_growth" || *st.Country != "SV" {
		t.Fatalf("state = %+v", st)
	}
	if len(st.Members) != 2 || st.Members[0].Role != model.Owner || st.Members[1].Account != "sub-2" {
		t.Fatalf("members = %+v", st.Members)
	}
	if *st.Limits.Issuers != 10 || st.Limits.TestSignatures != nil || *st.ProdHardCap != 50000 || st.TestHardCap != nil {
		t.Fatalf("limits = %+v", st.Limits)
	}
	if st.Subscription == nil || st.Subscription.Status != "active" || *st.Subscription.Interval != "month" || st.Subscription.CurrentPeriodEnd == nil {
		t.Fatalf("subscription = %+v", st.Subscription)
	}

	bad := []string{
		`{}`, `[]`, `not json`,
		`{"workspace_id": "x", "version": 1, "plan": "GOLD", "limits": {}}`,
		// A limit missing.
		`{"workspace_id": "x", "version": 1, "plan": "FREE", "limits": {"issuers": 1}}`,
		// A negative limit.
		`{"workspace_id": "x", "version": 1, "plan": "FREE", "limits": {"issuers": -1, "prod_signatures": null, "test_signatures": null, "members": null, "requests_per_minute": null, "retention_days": null}}`,
		// A bad role.
		`{"workspace_id": "x", "version": 1, "plan": "FREE", "members": [{"account": "a", "role": "KING"}], "limits": {"issuers": null, "prod_signatures": null, "test_signatures": null, "members": null, "requests_per_minute": null, "retention_days": null}}`,
	}
	for _, b := range bad {
		if _, err := ParseState([]byte(b)); err == nil {
			t.Errorf("accepted %s", b)
		}
	}
}

func TestIsPush(t *testing.T) {
	if !IsPush("Bearer s3cret", "s3cret") || !IsPush("bearer s3cret", "s3cret") {
		t.Fatal("refused the right token")
	}
	for _, h := range []string{"", "Bearer", "Bearer other", "Basic s3cret", "s3cret"} {
		if IsPush(h, "s3cret") {
			t.Errorf("accepted %q", h)
		}
	}
	if IsPush("Bearer ", "") {
		t.Fatal("an empty token accepted")
	}
}

func TestClient(t *testing.T) {
	var reported []UsageEntry
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/billing/sign/workspaces/org-1":
			_, _ = io.WriteString(w, stateBody)
		case r.Method == http.MethodPut && r.URL.Path == "/billing/sign/workspaces/gone":
			w.WriteHeader(http.StatusGone)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/billing/sign/usage":
			var b struct {
				Usage []UsageEntry `json:"usage"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			reported = append(reported, b.Usage...)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/", "tok")
	ctx := context.Background()
	if st, err := c.Activate(ctx, "org-1"); err != nil || st.Version != 7 {
		t.Fatalf("Activate = %+v, %v", st, err)
	}
	if _, err := c.Activate(ctx, "gone"); !errors.Is(err, ErrClosed) {
		t.Fatalf("gone: %v", err)
	}
	if _, err := c.Activate(ctx, "nope"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown: %v", err)
	}
	entries := make([]UsageEntry, ReportMaxEntries+5)
	for i := range entries {
		entries[i] = UsageEntry{WorkspaceID: "org-1", Period: "2026-11", Prod: int64(i)}
	}
	if err := c.ReportUsage(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if len(reported) != len(entries) {
		t.Fatalf("reported %d", len(reported))
	}
	if err := NewClient(srv.URL, "wrong").ReportUsage(ctx, entries[:1]); err == nil {
		t.Fatal("a refused report didn't fail")
	}
}

func TestMonthsToReport(t *testing.T) {
	got := model.MonthsToReport(time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC))
	if len(got) != 2 || got[0] != "2026-01" || got[1] != "2025-12" {
		t.Fatalf("early January: %v", got)
	}
	got = model.MonthsToReport(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	if len(got) != 1 || got[0] != "2026-11" {
		t.Fatalf("mid month: %v", got)
	}
}
