// Package mail sends the API's e-mail: through Resend's HTTP API when
// RESEND_API_KEY is set, and otherwise to the log, like the panel's
// src/lib/email.ts (the same variables, RESEND_API_KEY and EMAIL_FROM). No
// tracking: plain text and simple HTML, nothing else.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Message is one e-mail to one address.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// IdempotencyKey, if set, makes Resend send it once however many
	// times it's asked again with the same key and body, for 24 hours.
	IdempotencyKey string
}

// Sender sends a message. It returns nil once the mail service accepted
// it, an error wrapping ErrRejected if it refused that message for good,
// and any other error if it may take it later.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrRejected: the mail service refused the message itself (an invalid
// address, a missing field): sending it again won't change that.
var ErrRejected = errors.New("mail: rejected")

// ResendEndpoint is Resend's API for sending one e-mail.
const ResendEndpoint = "https://api.resend.com/emails"

// Resend sends through Resend's HTTP API.
type Resend struct {
	key, from string
	// Endpoint is where it posts; ResendEndpoint by default.
	Endpoint string
	Client   *http.Client
	// Interval is the least time between two requests: Resend allows a few
	// a second for the whole account, which the panel shares.
	Interval time.Duration

	mu   sync.Mutex
	last time.Time
}

// NewResend sends with key (RESEND_API_KEY) from `from` (EMAIL_FROM).
func NewResend(key, from string) *Resend {
	return &Resend{
		key: key, from: from, Endpoint: ResendEndpoint,
		Client:   &http.Client{Timeout: 15 * time.Second},
		Interval: 600 * time.Millisecond,
	}
}

// Send posts m to Resend. A 4xx about the message (400, 422) is
// ErrRejected; one about the key, the sender's domain or the rate (401,
// 403, 429) and a 5xx are errors to retry. A 409 because the same key came
// with another body means a message with that key was already taken: it
// counts as sent.
func (r *Resend) Send(ctx context.Context, m Message) error {
	if err := r.wait(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
		HTML    string   `json:"html,omitempty"`
	}{r.from, []string{m.To}, m.Subject, m.Text, m.HTML})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sello-sign-api")
	if m.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", m.IdempotencyKey)
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("mail: resend: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	// Only the error's name: its message may repeat the address.
	var problem struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &problem)
	switch {
	case res.StatusCode == http.StatusConflict && problem.Name == "invalid_idempotent_request":
		return nil
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: resend answered %d %s", ErrRejected, res.StatusCode, problem.Name)
	}
	return fmt.Errorf("mail: resend answered %d %s", res.StatusCode, problem.Name)
}

// wait keeps Interval between two requests.
func (r *Resend) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d := time.Until(r.last.Add(r.Interval)); d > 0 {
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	r.last = time.Now()
	return nil
}

// Log writes each message to the log instead of sending it: an install
// without RESEND_API_KEY. It always takes it.
type Log struct{}

// Send logs m.
func (Log) Send(ctx context.Context, m Message) error {
	slog.InfoContext(ctx, "e-mail not sent: RESEND_API_KEY is unset", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}
