// Package notices e-mails, every hour, what a space has to act on before
// signing stops, the same in the cloud and in a self-hosted install:
//
//   - an active certificate of one of its organizations that expires in 30
//     days, in 7 or in 1, or that expired while still active: only the
//     most pressing threshold due, never one less pressing after it (a
//     certificate uploaded 5 days before it expires gets the 7 days' notice
//     alone), all of a space's due in one e-mail;
//   - what it signed this month in an environment reaching 80 % of the
//     quota of its plan, the whole quota when signing goes on past it (a
//     paid plan's overage in production, its margin in test), and the cap
//     where it stops (UsageThreshold): only the highest reached.
//
// They go to who administers the space (its owners and admins), at the
// address the panel's token gave for each, through internal/mail. Each
// notice goes once: a row (migration 0004) is written only after the mail
// service took it for every recipient (or refused an address for good and
// took it for another); otherwise it's tried again the next hour, with the
// same idempotency key, so whoever got it doesn't get it twice. One
// instance runs at a time, under an advisory lock, like the retention of
// the signing log.
//
// The organizations of a space are its own: a space that signs with
// another's organization by delegation isn't told of its certificates (the
// owner uploads them, and the delegate may not even read them). The space
// of Sello's platform (SIGN_PLATFORM_WORKSPACE_ID) gets no notices: its
// organizations' certificates are Sello's platform's to warn of, which
// tells each organization itself, and it has no plan. Neither does a space
// whose organization Sello closed, which no longer signs.
package notices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/mail"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// The thresholds of a certificate, as stored.
const (
	Days30  = "30d"
	Days7   = "7d"
	Days1   = "1d"
	Expired = "expired"
)

// Horizon is how long before it expires a certificate is first notified.
const Horizon = 30 * 24 * time.Hour

// certificateRank is how pressing each threshold is.
var certificateRank = map[string]int{Days30: 1, Days7: 2, Days1: 3, Expired: 4}

// CertificateThreshold is the most pressing threshold a certificate that
// expires at notAfter is past at now; ok is false while it's more than
// Horizon away.
func CertificateThreshold(notAfter, now time.Time) (threshold string, ok bool) {
	switch left := notAfter.Sub(now); {
	case left <= 0:
		return Expired, true
	case left <= 24*time.Hour:
		return Days1, true
	case left <= 7*24*time.Hour:
		return Days7, true
	case left <= Horizon:
		return Days30, true
	}
	return "", false
}

// certificateDue is the threshold to notify a certificate of at now, given
// those already notified: the most pressing one past, unless it or a more
// pressing one went out.
func certificateDue(notAfter, now time.Time, sent []string) (string, bool) {
	th, ok := CertificateThreshold(notAfter, now)
	if !ok {
		return "", false
	}
	for _, s := range sent {
		if certificateRank[s] >= certificateRank[th] {
			return "", false
		}
	}
	return th, true
}

// The thresholds of a space's monthly usage, as stored.
const (
	// QuotaWarning: WarningPercent of the quota.
	QuotaWarning = "quota-80"
	// QuotaReached: the whole quota, when signing goes on past it (the cap
	// is above the quota).
	QuotaReached = "quota"
	// CapReached: where signing stops until the next month.
	CapReached = "cap"
)

// WarningPercent is the share of the quota that's first notified.
const WarningPercent = 80

// UsageThreshold is the highest threshold that used signatures reached
// against a quota (what the plan includes) and a cap (where signing
// stops; without one, the quota), and the limit it's measured against;
// ok is false if none, or nothing was signed.
func UsageThreshold(used int64, quota, cap *int) (threshold string, limit int, ok bool) {
	if quota == nil {
		quota = cap
	}
	if cap == nil {
		cap = quota
	}
	if used <= 0 || cap == nil {
		return "", 0, false
	}
	q, c := *quota, *cap
	switch {
	case used >= int64(c):
		return CapReached, c, true
	case c > q && used >= int64(q):
		return QuotaReached, q, true
	case q > 0 && used*100 >= int64(q)*WarningPercent:
		return QuotaWarning, q, true
	}
	return "", 0, false
}

// Source is the database the notices read and record.
type Source interface {
	TryLock(ctx context.Context, key int64, fn func(context.Context) error) (bool, error)
	ExpiringCertificates(ctx context.Context, before time.Time, except string) ([]store.ExpiringCertificate, error)
	MarkCertificateNotices(ctx context.Context, sent []store.CertificateNotice) error
	NoticeUsage(ctx context.Context, month, except string) ([]store.SpaceUsage, error)
	UsageNoticesSent(ctx context.Context, month string) ([]store.UsageNotice, error)
	MarkUsageNotice(ctx context.Context, n store.UsageNotice) error
	PurgeUsageNotices(ctx context.Context, before string) (int64, error)
	NoticeRecipients(ctx context.Context, workspaceID string) ([]string, error)
}

// lock makes one instance send the notices at a time; the migrations take
// 0x5e110_5167, the usage report 0x5e110_5168 and the retention
// 0x5e110_5169.
const lock int64 = 0x5e110_516e

// Every is how often the notices are looked for.
const Every = time.Hour

// Notifier sends the notices.
type Notifier struct {
	Source Source
	Mail   mail.Sender
	// PanelURL is the panel's origin (PANEL_URL), for the links.
	PanelURL string
	// Platform is the space of Sello's platform
	// (SIGN_PLATFORM_WORKSPACE_ID), which gets no notices; "" is none.
	Platform string
}

// Result is what a run did.
type Result struct {
	// Sent are the notices that went out and were recorded: a threshold
	// of a certificate, or of a space's usage.
	Sent int
	// Pending are those due that didn't: nobody to send them to, or the
	// mail service didn't take them. The next run tries again.
	Pending int
}

// Once sends what's due at now if no other instance is doing it: ran is
// false if one was. An error of the database stops the run; one of the
// mail service only leaves its notice pending.
func (n *Notifier) Once(ctx context.Context, now time.Time) (ran bool, res Result, err error) {
	ran, err = n.Source.TryLock(ctx, lock, func(ctx context.Context) error {
		if err := n.certificates(ctx, now, &res); err != nil {
			return err
		}
		if err := n.usage(ctx, now, &res); err != nil {
			return err
		}
		// Only the current month's are read; the one before stays a
		// while, around its end.
		first := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		_, err := n.Source.PurgeUsageNotices(ctx, model.Month(first.AddDate(0, -1, 0)))
		return err
	})
	return ran, res, err
}

// Run looks for notices a minute after starting and then every hour,
// until ctx ends.
func (n *Notifier) Run(ctx context.Context, now func() time.Time) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			ran, res, err := n.Once(ctx, now())
			switch {
			case err != nil && ctx.Err() == nil:
				slog.WarnContext(ctx, "could not send the notices", "sent", res.Sent, "pending", res.Pending, "error", err)
			case !ran:
				slog.DebugContext(ctx, "another instance is sending the notices")
			case res.Sent > 0 || res.Pending > 0:
				slog.InfoContext(ctx, "notices", "sent", res.Sent, "pending", res.Pending)
			}
			timer.Reset(Every)
		}
	}
}

// certificates notifies, space by space, the certificates whose threshold
// is due.
func (n *Notifier) certificates(ctx context.Context, now time.Time, res *Result) error {
	certs, err := n.Source.ExpiringCertificates(ctx, now.Add(Horizon), n.Platform)
	if err != nil {
		return err
	}
	for i := 0; i < len(certs); {
		ws := certs[i].WorkspaceID
		var due []certificateItem
		for ; i < len(certs) && certs[i].WorkspaceID == ws; i++ {
			if th, ok := certificateDue(certs[i].NotAfter, now, certs[i].Sent); ok {
				due = append(due, certificateItem{certs[i], th})
			}
		}
		if len(due) == 0 {
			continue
		}
		keys := make([]string, len(due))
		marks := make([]store.CertificateNotice, len(due))
		for j, d := range due {
			keys[j] = d.ID + "/" + d.Threshold
			marks[j] = store.CertificateNotice{CertificateID: d.ID, Threshold: d.Threshold}
		}
		key := "certificates/" + ws + "/" + strings.Join(keys, ",")
		ok, err := n.deliver(ctx, ws, key, certificateMessage(n.PanelURL, due))
		if err != nil {
			return err
		}
		if !ok {
			res.Pending += len(due)
			continue
		}
		if err := n.Source.MarkCertificateNotices(ctx, marks); err != nil {
			return err
		}
		res.Sent += len(due)
	}
	return nil
}

// usage notifies the spaces whose monthly usage reached a threshold.
func (n *Notifier) usage(ctx context.Context, now time.Time, res *Result) error {
	month := model.Month(now)
	rows, err := n.Source.UsageNoticesSent(ctx, month)
	if err != nil {
		return err
	}
	sent := map[store.UsageNotice]bool{}
	for _, r := range rows {
		sent[r] = true
	}
	usage, err := n.Source.NoticeUsage(ctx, month, n.Platform)
	if err != nil {
		return err
	}
	for _, u := range usage {
		th, limit, ok := UsageThreshold(u.Used, u.Quota, u.Cap)
		if !ok {
			continue
		}
		notice := store.UsageNotice{WorkspaceID: u.WorkspaceID, Environment: u.Environment, Month: month, Threshold: th, Limit: limit}
		if sent[notice] {
			continue
		}
		key := "usage/" + u.WorkspaceID + "/" + string(u.Environment) + "/" + month + "/" + th + "/" + itoa(limit)
		ok, err := n.deliver(ctx, u.WorkspaceID, key, usageMessage(n.PanelURL, u, th, month))
		if err != nil {
			return err
		}
		if !ok {
			res.Pending++
			continue
		}
		if err := n.Source.MarkUsageNotice(ctx, notice); err != nil {
			return err
		}
		res.Sent++
	}
	return nil
}

// deliver sends msg to who administers the space: delivered is true once
// every one of them took it, or the mail service refused an address for
// good and took it for another. A recipient who already took it isn't sent
// it again when it's retried: the idempotency key is the same.
func (n *Notifier) deliver(ctx context.Context, workspaceID, key string, msg mail.Message) (delivered bool, err error) {
	recipients, err := n.Source.NoticeRecipients(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if len(recipients) == 0 {
		slog.DebugContext(ctx, "a notice has nobody to go to", "workspace_id", workspaceID, "notice", key)
		return false, nil
	}
	accepted, rejected := 0, 0
	for _, to := range recipients {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		m := msg
		m.To, m.IdempotencyKey = to, idempotencyKey(key, to)
		switch err := n.Mail.Send(ctx, m); {
		case err == nil:
			accepted++
		case errors.Is(err, mail.ErrRejected):
			rejected++
			slog.WarnContext(ctx, "a recipient of a notice was refused", "workspace_id", workspaceID, "notice", key, "error", err)
		default:
			slog.WarnContext(ctx, "could not send a notice; it's tried again the next hour", "workspace_id", workspaceID, "notice", key, "error", err)
		}
	}
	return accepted > 0 && accepted+rejected == len(recipients), nil
}

// idempotencyKey is the same for a notice and a recipient in every run.
func idempotencyKey(key, to string) string {
	sum := sha256.Sum256([]byte(key + "\n" + strings.ToLower(to)))
	return "sign-notice-" + hex.EncodeToString(sum[:])
}
