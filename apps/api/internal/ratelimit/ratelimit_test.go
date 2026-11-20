package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func exercise(t *testing.T, l Limiter, prefix string) {
	t.Helper()
	ctx := context.Background()
	// 10:00:15 in some minute.
	now := time.Date(2026, 11, 20, 10, 0, 15, 0, time.UTC)
	key := Bucket{Name: prefix + "key", Limit: 2}
	space := Bucket{Name: prefix + "space", Limit: 3}

	res, err := l.Take(ctx, now, key, space)
	if err != nil || !res.Allowed || res.Bucket != key || res.Remaining != 1 || res.Reset != 45*time.Second {
		t.Fatalf("first: %+v %v", res, err)
	}
	if res, _ = l.Take(ctx, now, key, space); !res.Allowed || res.Remaining != 0 {
		t.Fatalf("second: %+v", res)
	}
	// The key is out; the space still has one.
	if res, _ = l.Take(ctx, now, key, space); res.Allowed || res.Bucket != key {
		t.Fatalf("third: %+v", res)
	}
	// Another key of the space: the refused request counted in the space
	// too, so it's out as well.
	other := Bucket{Name: prefix + "other", Limit: 10}
	if res, _ = l.Take(ctx, now, other, space); res.Allowed || res.Bucket != space {
		t.Fatalf("the space: %+v", res)
	}
	// The next minute starts again.
	later := now.Add(45 * time.Second)
	if res, _ = l.Take(ctx, later, key, space); !res.Allowed || res.Reset != time.Minute {
		t.Fatalf("next minute: %+v", res)
	}
	// No bucket: allowed.
	if res, _ = l.Take(ctx, later); !res.Allowed {
		t.Fatalf("no bucket: %+v", res)
	}
}

func TestMemory(t *testing.T) { exercise(t, NewMemory(), "") }

// TestValkey runs against VALKEY_TEST_URL (bun run test:pg sets the
// compose.yml's); without it, it's skipped.
func TestValkey(t *testing.T) {
	url := os.Getenv("VALKEY_TEST_URL")
	if url == "" {
		t.Skip("VALKEY_TEST_URL is unset")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	c := redis.NewClient(opts)
	defer c.Close()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	prefix := "test-" + hex.EncodeToString(b) + ":"
	exercise(t, NewValkey(c), prefix)
	ttl, err := c.TTL(context.Background(), "sign:rl:"+prefix+"key:"+"29919480").Result()
	if err != nil || ttl <= 0 || ttl > 2*Window {
		t.Fatalf("the window's key lasts %v (%v)", ttl, err)
	}
}
