// Package ratelimit is the signing rate limit: requests per minute, by
// space and by key, in fixed one-minute windows. In Valkey every instance
// shares the counts; in memory, without it, each instance counts its own.
package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Window is how long a count lasts: each minute starts from zero.
const Window = time.Minute

// Bucket is one limit a request counts against: its name (what it limits,
// like "workspace:<id>") and how many requests a window allows.
type Bucket struct {
	Name  string
	Limit int
}

// Result is what Take decided.
type Result struct {
	Allowed bool
	// Bucket is the one that refused or, when allowed, the one with the
	// fewest requests left.
	Bucket Bucket
	// Remaining is what Bucket allows in this window after this request.
	Remaining int
	// Reset is how long until the window ends.
	Reset time.Duration
}

// Limiter counts requests.
type Limiter interface {
	// Take counts one request in every bucket and says whether all of them
	// still allow it. A refused request counts too.
	Take(ctx context.Context, now time.Time, buckets ...Bucket) (Result, error)
}

func window(now time.Time) int64 { return now.UnixMilli() / Window.Milliseconds() }

func resetOf(now time.Time) time.Duration {
	return Window - time.Duration(now.UnixMilli()%Window.Milliseconds())*time.Millisecond
}

// decide reads the counts of buckets (in their order) after the request.
func decide(now time.Time, buckets []Bucket, counts []int64) Result {
	res := Result{Allowed: true, Reset: resetOf(now)}
	for i, b := range buckets {
		left := b.Limit - int(counts[i])
		if left < 0 {
			return Result{Allowed: false, Bucket: b, Remaining: 0, Reset: res.Reset}
		}
		if i == 0 || left < res.Remaining {
			res.Bucket, res.Remaining = b, left
		}
	}
	return res
}

// NewValkey counts in Valkey, under sign:rl:<bucket>:<window>.
func NewValkey(c *redis.Client) Limiter { return &valkey{c: c} }

type valkey struct{ c *redis.Client }

func (v *valkey) Take(ctx context.Context, now time.Time, buckets ...Bucket) (Result, error) {
	if len(buckets) == 0 {
		return Result{Allowed: true, Reset: resetOf(now)}, nil
	}
	w := strconv.FormatInt(window(now), 10)
	incrs := make([]*redis.IntCmd, len(buckets))
	_, err := v.c.Pipelined(ctx, func(p redis.Pipeliner) error {
		for i, b := range buckets {
			key := "sign:rl:" + b.Name + ":" + w
			incrs[i] = p.Incr(ctx, key)
			// Two windows: the key names its own, so a later EXPIRE only
			// keeps it a little longer.
			p.Expire(ctx, key, 2*Window)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	counts := make([]int64, len(buckets))
	for i, c := range incrs {
		counts[i] = c.Val()
	}
	return decide(now, buckets, counts), nil
}

// NewMemory counts in this process: with more than one instance, each
// allows the whole limit.
func NewMemory() Limiter { return &memory{counts: map[string]count{}} }

type count struct {
	window int64
	n      int64
}

type memory struct {
	mu     sync.Mutex
	counts map[string]count
}

func (m *memory) Take(_ context.Context, now time.Time, buckets ...Bucket) (Result, error) {
	w := window(now)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.counts) > 100_000 {
		for k, c := range m.counts {
			if c.window != w {
				delete(m.counts, k)
			}
		}
	}
	counts := make([]int64, len(buckets))
	for i, b := range buckets {
		c := m.counts[b.Name]
		if c.window != w {
			c = count{window: w}
		}
		c.n++
		m.counts[b.Name] = c
		counts[i] = c.n
	}
	return decide(now, buckets, counts), nil
}
