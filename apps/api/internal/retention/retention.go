// Package retention deletes, every hour, what the signing log keeps past
// the retention of the space that signed: its plan's (retention_days,
// which billing pushes) or, in a space without a plan, the install's
// (SIGN_LOG_RETENTION_DAYS) or the console's. One instance at a time, under
// an advisory lock, like the usage report to billing.
package retention

import (
	"context"
	"log/slog"
	"time"
)

// Source is where the log is purged.
type Source interface {
	PurgeExpiredSignatures(ctx context.Context, now time.Time) (int64, error)
	TryLock(ctx context.Context, key int64, fn func(context.Context) error) (bool, error)
}

// lock makes one instance purge at a time; the migrations take
// 0x5e110_5167 and the usage report 0x5e110_5168.
const lock int64 = 0x5e110_5169

// Every is how often the log is purged.
const Every = time.Hour

// Once purges the log if no other instance is doing it: ran is false if
// one was.
func Once(ctx context.Context, src Source, now time.Time) (ran bool, deleted int64, err error) {
	ran, err = src.TryLock(ctx, lock, func(ctx context.Context) error {
		var err error
		deleted, err = src.PurgeExpiredSignatures(ctx, now)
		return err
	})
	return ran, deleted, err
}

// Run purges the log a minute after starting and then every hour, until
// ctx ends.
func Run(ctx context.Context, src Source, now func() time.Time) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			ran, deleted, err := Once(ctx, src, now())
			switch {
			case err != nil && ctx.Err() == nil:
				slog.WarnContext(ctx, "could not purge the signing log", "deleted", deleted, "error", err)
			case !ran:
				slog.DebugContext(ctx, "another instance is purging the signing log")
			case deleted > 0:
				slog.InfoContext(ctx, "signing log purged", "deleted", deleted)
			}
			timer.Reset(Every)
		}
	}
}
