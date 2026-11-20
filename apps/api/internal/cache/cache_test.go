package cache

import (
	"context"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/signing"
)

func TestMaterialLRU(t *testing.T) {
	now := time.Unix(0, 0)
	c := NewMaterial(2, time.Minute)
	c.now = func() time.Time { return now }
	c.Put("a", signing.Material{})
	c.Put("b", signing.Material{})
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	c.Put("c", signing.Material{}) // evicts b, the least recently used
	if _, ok := c.Get("b"); ok {
		t.Fatal("b kept beyond the size")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a evicted")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("a kept beyond its TTL")
	}
	c.Delete("c")
	if c.Len() != 0 {
		t.Fatalf("len = %d", c.Len())
	}
}

func TestMemoryKeys(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(0, 0)
	k := NewMemoryKeys().(*memoryKeys)
	k.now = func() time.Time { return now }
	k.Set(ctx, []byte("h"), auth.Key{ID: "1"})
	if got, ok := k.Get(ctx, []byte("h")); !ok || got.ID != "1" {
		t.Fatal("not kept")
	}
	k.Delete(ctx, []byte("h"))
	if _, ok := k.Get(ctx, []byte("h")); ok {
		t.Fatal("kept after delete")
	}
	k.Set(ctx, []byte("h"), auth.Key{ID: "1"})
	now = now.Add(KeyTTL + time.Second)
	if _, ok := k.Get(ctx, []byte("h")); ok {
		t.Fatal("kept beyond the TTL")
	}
}
