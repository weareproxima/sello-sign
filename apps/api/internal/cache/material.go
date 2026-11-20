package cache

import (
	"container/list"
	"sync"
	"time"

	"proxima.la/sello/sign/internal/signing"
)

// Material keeps decrypted certificate material in memory, by certificate
// id, so signing doesn't decrypt and parse the PEM every time. A
// certificate's material never changes, so an entry is never stale: the
// TTL only bounds how long a private key stays in memory after its last
// use, and the size how many.
type Material struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	ll   *list.List
	byID map[string]*list.Element
	now  func() time.Time
}

type materialEntry struct {
	id      string
	m       signing.Material
	expires time.Time
}

// NewMaterial keeps at most max entries, each for ttl after it was put.
func NewMaterial(max int, ttl time.Duration) *Material {
	return &Material{ttl: ttl, max: max, ll: list.New(), byID: map[string]*list.Element{}, now: time.Now}
}

// Get returns the material of a certificate if it's kept.
func (c *Material) Get(id string) (signing.Material, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.byID[id]
	if !ok {
		return signing.Material{}, false
	}
	e := el.Value.(*materialEntry)
	if c.now().After(e.expires) {
		c.ll.Remove(el)
		delete(c.byID, id)
		return signing.Material{}, false
	}
	c.ll.MoveToFront(el)
	return e.m, true
}

// Put keeps a certificate's material.
func (c *Material) Put(id string, m signing.Material) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byID[id]; ok {
		e := el.Value.(*materialEntry)
		e.m, e.expires = m, c.now().Add(c.ttl)
		c.ll.MoveToFront(el)
		return
	}
	c.byID[id] = c.ll.PushFront(&materialEntry{id: id, m: m, expires: c.now().Add(c.ttl)})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.byID, last.Value.(*materialEntry).id)
	}
}

// Delete forgets a certificate's material (it was deleted).
func (c *Material) Delete(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byID[id]; ok {
		c.ll.Remove(el)
		delete(c.byID, id)
	}
}

// Len is how many entries are kept.
func (c *Material) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
