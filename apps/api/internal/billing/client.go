package billing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/model"
)

// Timeout is how long a call to billing may take.
const Timeout = 10 * time.Second

// ReportMaxEntries is billing's limit of months in one usage report.
const ReportMaxEntries = 1000

var (
	// ErrUnknown: billing doesn't know the organization yet (the IdP tells
	// it as it creates it: a moment later it will).
	ErrUnknown = errors.New("billing: unknown organization")
	// ErrClosed: the organization was deleted.
	ErrClosed = errors.New("billing: organization closed")
	// ErrUnavailable: billing didn't answer, or answered nonsense.
	ErrUnavailable = errors.New("billing: unavailable")
)

// Client calls billing at BILLING_URL with BILLING_TOKEN.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient calls billing at base with token.
func NewClient(base, token string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: Timeout}}
}

// WithHTTPClient makes the calls with h (a test's).
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.http = h
	return c
}

func (c *Client) call(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

// Activate activates the space of the organization id at billing, which
// from then on pushes it every change, and returns its state. Asked again,
// the same.
func (c *Client) Activate(ctx context.Context, id string) (State, error) {
	res, err := c.call(ctx, http.MethodPut, "/billing/sign/workspaces/"+url.PathEscape(id), nil)
	if err != nil {
		slog.WarnContext(ctx, "billing did not answer the activation of a space", "sello_workspace_id", id, "error", err)
		return State{}, ErrUnavailable
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	switch {
	case res.StatusCode == http.StatusNotFound:
		return State{}, ErrUnknown
	case res.StatusCode == http.StatusGone:
		return State{}, ErrClosed
	case res.StatusCode != http.StatusOK:
		slog.WarnContext(ctx, "billing refused to activate a space", "sello_workspace_id", id, "status", res.StatusCode)
		return State{}, ErrUnavailable
	}
	st, err := ParseState(body)
	if err != nil || st.WorkspaceID != id {
		return State{}, ErrUnavailable
	}
	return st, nil
}

// ReportUsage sends billing what each space signed, in chunks of
// ReportMaxEntries.
func (c *Client) ReportUsage(ctx context.Context, entries []UsageEntry) error {
	for i := 0; i < len(entries); i += ReportMaxEntries {
		chunk := entries[i:min(i+ReportMaxEntries, len(entries))]
		res, err := c.call(ctx, http.MethodPost, "/billing/sign/usage", map[string]any{"usage": chunk})
		if err != nil {
			return fmt.Errorf("billing: usage report: %w", err)
		}
		snippet, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("billing refused the usage report: %d %s", res.StatusCode, strings.TrimSpace(string(snippet)))
		}
	}
	return nil
}

// IsPush says whether header is "Bearer <token>", compared in a time that
// doesn't depend on where they differ.
func IsPush(header, token string) bool {
	if token == "" {
		return false
	}
	scheme, rest, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(rest)))
	want := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// UsageSource is where the reporter reads the counters.
type UsageSource interface {
	UsageReport(ctx context.Context, months []string) ([]UsageEntry, error)
	TryLock(ctx context.Context, key int64, fn func(context.Context) error) (bool, error)
}

// reportLock makes one instance report at a time.
const reportLock int64 = 0x5e110_5168

// ReportEvery is how often the usage is reported.
const ReportEvery = time.Hour

// RunReporter reports every hour, from the counters, what each space of
// the cloud signed this month (and the month before, its first days): the
// whole count each time, which only grows. It returns when ctx ends.
func RunReporter(ctx context.Context, c *Client, src UsageSource, now func() time.Time) {
	report := func() {
		ran, err := src.TryLock(ctx, reportLock, func(ctx context.Context) error {
			entries, err := src.UsageReport(ctx, model.MonthsToReport(now()))
			if err != nil {
				return err
			}
			if err := c.ReportUsage(ctx, entries); err != nil {
				return err
			}
			slog.InfoContext(ctx, "usage reported to billing", "months", len(entries))
			return nil
		})
		if err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "could not report the usage to billing", "error", err)
		} else if !ran {
			slog.DebugContext(ctx, "another instance is reporting the usage")
		}
	}
	// A first report a minute after starting, then every hour.
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			report()
			timer.Reset(ReportEvery)
		}
	}
}
