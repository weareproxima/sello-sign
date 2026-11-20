// Package secrets seals what the API keeps encrypted at rest: the material
// of each certificate (its private key above all). AES-256-GCM, with the
// version of the key that sealed it in the first byte, so the key can change
// without re-encrypting everything at once: a sealed value is
// version ‖ nonce ‖ ciphertext+tag, and the caller's associated data (the
// certificate's id) binds it to its row.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// KeySize is AES-256's.
const KeySize = 32

var (
	// ErrUnknownVersion: the value was sealed with a key the keyring
	// doesn't have.
	ErrUnknownVersion = errors.New("secrets: sealed with an unknown key")
	// ErrCorrupt: the value isn't one Seal wrote, or was changed, or belongs
	// to another row.
	ErrCorrupt = errors.New("secrets: cannot open the sealed value")
)

// Keyring seals with its current key and opens with any of its keys.
type Keyring struct {
	current byte
	aeads   map[byte]cipher.AEAD
}

// ParseKey reads a key as SIGN_SECRETS_KEY writes it: 32 bytes in base64
// (standard or URL, with or without padding), optionally preceded by its
// version, "v2:…". Without one, the version is 1. The error never echoes
// the key.
func ParseKey(s string) (version byte, key []byte, err error) {
	s = strings.TrimSpace(s)
	version = 1
	if v, rest, ok := strings.Cut(s, ":"); ok && strings.HasPrefix(v, "v") {
		n, perr := strconv.Atoi(v[1:])
		if perr != nil || n < 1 || n > 255 {
			return 0, nil, errors.New("secrets: the key's version must be v1 to v255")
		}
		version, s = byte(n), rest
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, derr := enc.DecodeString(s); derr == nil {
			if len(b) != KeySize {
				return 0, nil, fmt.Errorf("secrets: the key must be %d bytes, not %d", KeySize, len(b))
			}
			return version, b, nil
		}
	}
	return 0, nil, errors.New("secrets: the key is not base64")
}

// NewKeyring seals with current and opens with it and previous (the keys a
// rotation leaves behind), each as ParseKey reads it.
func NewKeyring(current string, previous ...string) (*Keyring, error) {
	k := &Keyring{aeads: map[byte]cipher.AEAD{}}
	v, key, err := ParseKey(current)
	if err != nil {
		return nil, err
	}
	if err := k.add(v, key); err != nil {
		return nil, err
	}
	k.current = v
	for _, p := range previous {
		if strings.TrimSpace(p) == "" {
			continue
		}
		pv, pkey, err := ParseKey(p)
		if err != nil {
			return nil, err
		}
		if _, dup := k.aeads[pv]; dup {
			return nil, fmt.Errorf("secrets: two keys with version %d", pv)
		}
		if err := k.add(pv, pkey); err != nil {
			return nil, err
		}
	}
	return k, nil
}

func (k *Keyring) add(v byte, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	k.aeads[v] = aead
	return nil
}

// Seal encrypts plaintext for the row ad names.
func (k *Keyring) Seal(plaintext, ad []byte) ([]byte, error) {
	aead := k.aeads[k.current]
	out := make([]byte, 1+aead.NonceSize(), 1+aead.NonceSize()+len(plaintext)+aead.Overhead())
	out[0] = k.current
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, err
	}
	return aead.Seal(out, out[1:], plaintext, ad), nil
}

// Open decrypts what Seal wrote for the same ad.
func (k *Keyring) Open(sealed, ad []byte) ([]byte, error) {
	if len(sealed) < 1 {
		return nil, ErrCorrupt
	}
	aead, ok := k.aeads[sealed[0]]
	if !ok {
		return nil, ErrUnknownVersion
	}
	n := aead.NonceSize()
	if len(sealed) < 1+n+aead.Overhead() {
		return nil, ErrCorrupt
	}
	plain, err := aead.Open(nil, sealed[1:1+n], sealed[1+n:], ad)
	if err != nil {
		return nil, ErrCorrupt
	}
	return plain, nil
}

// Current is the version Seal writes.
func (k *Keyring) Current() byte { return k.current }

// Version is the version of the key that sealed a value.
func Version(sealed []byte) (byte, bool) {
	if len(sealed) == 0 {
		return 0, false
	}
	return sealed[0], true
}
