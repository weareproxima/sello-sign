package mail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeResend answers like Resend's POST /emails: status and body for each
// request, in turn, and keeps what it was sent.
type fakeResend struct {
	mu       sync.Mutex
	answers  []answer
	requests []*http.Request
	bodies   []map[string]any
}

type answer struct {
	status int
	body   string
}

func (f *fakeResend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.requests = append(f.requests, r)
	f.bodies = append(f.bodies, body)
	a := answer{200, `{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`}
	if len(f.answers) > 0 {
		a, f.answers = f.answers[0], f.answers[1:]
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(a.status)
	_, _ = w.Write([]byte(a.body))
}

func newFake(t *testing.T, answers ...answer) (*fakeResend, *Resend) {
	t.Helper()
	f := &fakeResend{answers: answers}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	r := NewResend("re_test_key", "Sello Sign <noreply@sign.sello.tax>")
	r.Endpoint, r.Interval = srv.URL+"/emails", 0
	return f, r
}

var message = Message{
	To: "ana@example.com", Subject: "Asunto", Text: "Texto.\n", HTML: "<p>Texto.</p>\n", IdempotencyKey: "sign-notice-abc",
}

func TestResendSends(t *testing.T) {
	f, r := newFake(t)
	if err := r.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	req, body := f.requests[0], f.bodies[0]
	if req.Method != http.MethodPost || req.URL.Path != "/emails" {
		t.Fatalf("%s %s", req.Method, req.URL.Path)
	}
	for h, want := range map[string]string{
		"Authorization":   "Bearer re_test_key",
		"Content-Type":    "application/json",
		"Idempotency-Key": "sign-notice-abc",
	} {
		if got := req.Header.Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
	to, _ := body["to"].([]any)
	if body["from"] != "Sello Sign <noreply@sign.sello.tax>" || len(to) != 1 || to[0] != "ana@example.com" ||
		body["subject"] != "Asunto" || body["text"] != "Texto.\n" || body["html"] != "<p>Texto.</p>\n" || len(body) != 5 {
		t.Fatalf("body %v", body)
	}

	// Without HTML nor key, neither goes.
	if err := r.Send(context.Background(), Message{To: "bob@example.com", Subject: "S", Text: "T"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.bodies[1]["html"]; ok || f.requests[1].Header.Get("Idempotency-Key") != "" {
		t.Fatalf("html or key sent: %v %v", f.bodies[1], f.requests[1].Header)
	}
}

func TestResendAnswers(t *testing.T) {
	for _, c := range []struct {
		name     string
		answer   answer
		ok       bool
		rejected bool
	}{
		{"accepted", answer{200, `{"id":"x"}`}, true, false},
		{"an invalid address", answer{422, `{"statusCode":422,"name":"invalid_parameter","message":"Invalid to: ana@"}`}, false, true},
		{"a validation error", answer{400, `{"statusCode":400,"name":"validation_error","message":"…"}`}, false, true},
		{"the same key with another body: already sent", answer{409, `{"statusCode":409,"name":"invalid_idempotent_request","message":"…"}`}, true, false},
		{"the same key in flight", answer{409, `{"statusCode":409,"name":"concurrent_idempotent_requests","message":"…"}`}, false, false},
		{"a wrong key", answer{401, `{"statusCode":401,"name":"missing_api_key","message":"…"}`}, false, false},
		{"a domain not verified", answer{403, `{"statusCode":403,"name":"validation_error","message":"…"}`}, false, false},
		{"too fast", answer{429, `{"statusCode":429,"name":"rate_limit_exceeded","message":"…"}`}, false, false},
		{"down", answer{503, `<html>`}, false, false},
	} {
		_, r := newFake(t, c.answer)
		err := r.Send(context.Background(), message)
		if (err == nil) != c.ok || errors.Is(err, ErrRejected) != c.rejected {
			t.Errorf("%s: %v", c.name, err)
		}
		// Never the message of the error, which may repeat the address.
		if err != nil && strings.Contains(err.Error(), "ana@") {
			t.Errorf("%s: the error says the address: %v", c.name, err)
		}
	}
}

func TestResendUnreachable(t *testing.T) {
	r := NewResend("re_test_key", "x <a@b.c>")
	r.Endpoint, r.Interval = "http://127.0.0.1:1/emails", 0
	if err := r.Send(context.Background(), message); err == nil || errors.Is(err, ErrRejected) {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestResendKeepsItsInterval(t *testing.T) {
	_, r := newFake(t)
	r.Interval = 50 * time.Millisecond
	start := time.Now()
	for range 3 {
		if err := r.Send(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("three in %v", d)
	}
	// A context that ends while it waits stops it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Interval = time.Hour
	if err := r.Send(ctx, message); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func TestLogTakesIt(t *testing.T) {
	if err := (Log{}).Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
}
