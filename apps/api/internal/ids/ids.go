// Package ids makes and reads the API's identifiers: UUIDv7 (RFC 9562), in
// their canonical text form. They sort by creation time, which is what the
// keyset pagination relies on.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ID is a UUID's 16 bytes.
type ID [16]byte

var (
	mu   sync.Mutex
	last [16]byte
)

// New returns a new UUIDv7. Within one process the ids it returns are
// strictly increasing, also within the same millisecond: the 12 bits after
// the version count up from the previous id when the clock didn't move.
func New() string { return NewAt(time.Now()).String() }

// NewAt returns a UUIDv7 for the instant t.
func NewAt(t time.Time) ID {
	var u ID
	if _, err := rand.Read(u[:]); err != nil {
		panic(err)
	}
	ms := uint64(t.UnixMilli())
	u[0], u[1], u[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	u[3], u[4], u[5] = byte(ms>>16), byte(ms>>8), byte(ms)

	mu.Lock()
	// Same or earlier millisecond than the last one: bump the counter in
	// rand_a (12 bits) so ids stay ordered.
	if cmp(u[:6], last[:6]) <= 0 {
		copy(u[:6], last[:6])
		seq := (uint16(last[6]&0x0f)<<8 | uint16(last[7])) + 1
		if seq > 0x0fff {
			// The counter overflowed: move to the next millisecond.
			seq = 0
			for i := 5; i >= 0; i-- {
				u[i]++
				if u[i] != 0 {
					break
				}
			}
		}
		u[6], u[7] = byte(seq>>8), byte(seq)
	}
	u[6] = (u[6] & 0x0f) | 0x70
	u[8] = (u[8] & 0x3f) | 0x80
	last = u
	mu.Unlock()
	return u
}

func cmp(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// String is the canonical form: 8-4-4-4-12 lower-case hex.
func (u ID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}

// ErrInvalid is any string that isn't a UUID.
var ErrInvalid = errors.New("ids: not a UUID")

// Parse reads a UUID in its canonical form, in any case.
func Parse(s string) (ID, error) {
	var u ID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, ErrInvalid
	}
	src := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	if _, err := hex.Decode(u[:], []byte(src)); err != nil {
		return u, ErrInvalid
	}
	return u, nil
}

// Valid says whether s is a UUID.
func Valid(s string) bool {
	_, err := Parse(s)
	return err == nil
}

// Canonical returns s in lower case if it's a UUID, and ok.
func Canonical(s string) (string, bool) {
	u, err := Parse(s)
	if err != nil {
		return "", false
	}
	return u.String(), true
}
