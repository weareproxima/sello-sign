// Package cache keeps what the API reads on every request: the API keys
// (in Valkey, shared by every instance, or in memory without it) and the
// decrypted certificate material (only in this process's memory, never in
// Valkey: a private key never leaves the process unencrypted).
package cache

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"proxima.la/sello/sign/internal/auth"
)

// KeyTTL is how long a key's lookup is kept. Revoking a key drops it at
// once; the TTL bounds anything else (a key whose organization was deleted).
const KeyTTL = 5 * time.Minute

// Keys caches API key lookups by the key's hash: never the key itself.
type Keys interface {
	Get(ctx context.Context, hash []byte) (auth.Key, bool)
	Set(ctx context.Context, hash []byte, k auth.Key)
	Delete(ctx context.Context, hash []byte)
}

// NewValkeyKeys caches in Valkey under sign:apikey:<hex of the hash>.
func NewValkeyKeys(c *redis.Client) Keys { return &valkeyKeys{c: c} }

type valkeyKeys struct{ c *redis.Client }

func keyName(hash []byte) string { return "sign:apikey:" + hex.EncodeToString(hash) }

func (v *valkeyKeys) Get(ctx context.Context, hash []byte) (auth.Key, bool) {
	b, err := v.c.Get(ctx, keyName(hash)).Bytes()
	if err != nil {
		return auth.Key{}, false
	}
	var k auth.Key
	if json.Unmarshal(b, &k) != nil || k.ID == "" {
		return auth.Key{}, false
	}
	return k, true
}

func (v *valkeyKeys) Set(ctx context.Context, hash []byte, k auth.Key) {
	b, err := json.Marshal(k)
	if err != nil {
		return
	}
	_ = v.c.Set(ctx, keyName(hash), b, KeyTTL).Err()
}

func (v *valkeyKeys) Delete(ctx context.Context, hash []byte) {
	_ = v.c.Del(ctx, keyName(hash)).Err()
}

// OpenValkey connects to url (REDIS_URL) and checks it answers.
func OpenValkey(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("cache: REDIS_URL isn't a redis:// URL")
	}
	c := redis.NewClient(opts)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// NewMemoryKeys caches in this process, for an install without Valkey:
// with more than one instance, a revoked key may still work on another one
// until KeyTTL.
func NewMemoryKeys() Keys { return &memoryKeys{m: map[string]memoryKey{}, now: time.Now} }

type memoryKey struct {
	k       auth.Key
	expires time.Time
}

type memoryKeys struct {
	mu  sync.Mutex
	m   map[string]memoryKey
	now func() time.Time
}

func (m *memoryKeys) Get(_ context.Context, hash []byte) (auth.Key, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.m[string(hash)]
	if !ok || m.now().After(e.expires) {
		delete(m.m, string(hash))
		return auth.Key{}, false
	}
	return e.k, true
}

func (m *memoryKeys) Set(_ context.Context, hash []byte, k auth.Key) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.m) > 10_000 {
		clear(m.m)
	}
	m.m[string(hash)] = memoryKey{k: k, expires: m.now().Add(KeyTTL)}
}

func (m *memoryKeys) Delete(_ context.Context, hash []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, string(hash))
}
