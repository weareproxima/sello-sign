// Package retention deletes, every hour, what the signing log keeps past
// the retention of the space that signed: its plan's (retention_days,
// which billing pushes) or, in a space without a plan, the install's
// (SIGN_LOG_RETENTION_DAYS) or the console's; and the audit log's events
// older than audit.RetentionDays, whatever the plan. One instance at a
// time, under an advisory lock, like the usage report to billing.
package retention

import (
	"context"
	"log/slog"
	"time"
)

// Source is where the log is purged.
type Source interface {
	PurgeExpiredSignatures(ctx context.Context, now time.Time) (int64, error)
	PurgeExpiredAuditEvents(ctx context.Context, now time.Time) (int64, error)
	TryLock(ctx context.Context, key int64, fn func(context.Context) error) (bool, error)
}

// Purged is what one purge deleted.
type Purged struct {
	Signatures  int64
	AuditEvents int64
}

// lock makes one instance purge at a time; the migrations take
// 0x5e110_5167 and the usage report 0x5e110_5168.
const lock int64 = 0x5e110_5169

// Every is how often the logs are purged.
const Every = time.Hour

// Once purges the logs if no other instance is doing it: ran is false if
// one was.
func Once(ctx context.Context, src Source, now time.Time) (ran bool, purged Purged, err error) {
	ran, err = src.TryLock(ctx, lock, func(ctx context.Context) error {
		var err error
		if purged.Signatures, err = src.PurgeExpiredSignatures(ctx, now); err != nil {
			return err
		}
		purged.AuditEvents, err = src.PurgeExpiredAuditEvents(ctx, now)
		return err
	})
	return ran, purged, err
}

// Run purges the logs a minute after starting and then every hour, until
// ctx ends.
func Run(ctx context.Context, src Source, now func() time.Time) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			ran, purged, err := Once(ctx, src, now())
			switch {
			case err != nil && ctx.Err() == nil:
				slog.WarnContext(ctx, "could not purge the logs", "signatures", purged.Signatures,
					"audit_events", purged.AuditEvents, "error", err)
			case !ran:
				slog.DebugContext(ctx, "another instance is purging the logs")
			case purged.Signatures > 0 || purged.AuditEvents > 0:
				slog.InfoContext(ctx, "logs purged", "signatures", purged.Signatures, "audit_events", purged.AuditEvents)
			}
			timer.Reset(Every)
		}
	}
}
