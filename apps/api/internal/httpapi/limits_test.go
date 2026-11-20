package httpapi

import (
	"testing"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

func intp(n int) *int { return &n }

func TestRequestsPerMinuteOf(t *testing.T) {
	for _, c := range []struct {
		ceiling, own, want *int
	}{
		{nil, nil, nil},
		{intp(600), nil, intp(600)},
		{nil, intp(50), intp(50)},
		{intp(600), intp(50), intp(50)},
		// Billing lowered the plan below what the space had set.
		{intp(120), intp(500), intp(120)},
	} {
		ws := store.Workspace{OwnRequestsPerMinute: c.own}
		ws.Limits.RequestsPerMinute = c.ceiling
		got := requestsPerMinuteOf(ws)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Fatalf("ceiling %v own %v: %v", c.ceiling, c.own, got)
		}
	}
}

func TestMonthlyCap(t *testing.T) {
	var ws store.Workspace
	if monthlyCap(ws, model.Prod) != nil || monthlyCap(ws, model.Test) != nil {
		t.Fatal("no limits is no cap")
	}
	// Free: the quota is the cap.
	ws.Limits.ProdSignatures, ws.Limits.TestSignatures = intp(500), intp(5000)
	if *monthlyCap(ws, model.Prod) != 500 || *monthlyCap(ws, model.Test) != 5000 {
		t.Fatal("the quota")
	}
	// Pro: past the quota, up to the hard cap (the overage).
	ws.Limits.ProdHardCap, ws.Limits.TestHardCap = intp(1000), intp(5500)
	if *monthlyCap(ws, model.Prod) != 1000 || *monthlyCap(ws, model.Test) != 5500 {
		t.Fatal("the hard cap")
	}
}

func TestRateBuckets(t *testing.T) {
	ws := store.Workspace{ID: "w"}
	if len(rateBuckets(ws, nil)) != 0 || len(rateBuckets(ws, &auth.Key{ID: "k"})) != 0 {
		t.Fatal("no limits is no bucket")
	}
	ws.Limits.RequestsPerMinute = intp(600)
	b := rateBuckets(ws, &auth.Key{ID: "k", RequestsPerMinute: intp(10)})
	if len(b) != 2 || b[0].Name != "key:k" || b[0].Limit != 10 || b[1].Name != "workspace:w" || b[1].Limit != 600 {
		t.Fatalf("buckets = %+v", b)
	}
}

func TestNextMonth(t *testing.T) {
	got := nextMonth(time.Date(2026, 12, 31, 23, 59, 0, 0, time.FixedZone("CST", -6*3600)))
	if !got.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)) {
		// 23:59 CST on Dec 31 is already January in UTC.
		t.Fatalf("next month = %v", got)
	}
}
