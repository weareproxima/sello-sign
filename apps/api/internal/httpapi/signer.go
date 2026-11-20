package httpapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/cache"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/secrets"
	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/store"
)

var (
	// ErrCountryNotEnabled: the organization's country isn't offered here
	// (SIGN_COUNTRIES) or has no signer.
	ErrCountryNotEnabled = errors.New("country not enabled")
	// ErrNoActiveCertificate: the organization has no active certificate in
	// the environment.
	ErrNoActiveCertificate = errors.New("no active certificate")
	// ErrCertificateExpired: its active certificate expired.
	ErrCertificateExpired = errors.New("certificate expired")
)

// MaterialLoader reads an organization's active certificate in an
// environment with its sealed material (store.ErrNotFound if none).
type MaterialLoader func(ctx context.Context, organizationID string, env model.Environment) (store.Certificate, []byte, error)

// Signer signs and verifies with an organization's active certificate: it
// opens the sealed material (keeping it decrypted only in this process's
// memory) and hands it to the organization's country.
type Signer struct {
	keyring  *secrets.Keyring
	material *cache.Material
	registry *signing.Registry
	enabled  []string
	load     MaterialLoader
	now      func() time.Time
}

// NewSigner signs for the countries of registry that enabled lists.
func NewSigner(keyring *secrets.Keyring, material *cache.Material, registry *signing.Registry, enabled []string, load MaterialLoader) *Signer {
	up := make([]string, len(enabled))
	for i, c := range enabled {
		up[i] = strings.ToUpper(c)
	}
	return &Signer{keyring: keyring, material: material, registry: registry, enabled: up, load: load, now: time.Now}
}

// Country is the signer of an offered country.
func (s *Signer) Country(code string) (signing.Country, error) {
	if !slices.Contains(s.enabled, strings.ToUpper(code)) {
		return nil, ErrCountryNotEnabled
	}
	c, ok := s.registry.Lookup(code)
	if !ok {
		return nil, ErrCountryNotEnabled
	}
	return c, nil
}

// Seal encrypts a certificate's material for its row.
func (s *Signer) Seal(certificateID string, m signing.Material) ([]byte, error) {
	pem, err := signing.EncodeMaterial(m)
	if err != nil {
		return nil, err
	}
	defer clear(pem)
	return s.keyring.Seal(pem, []byte(certificateID))
}

// active loads the organization's active certificate in env and its
// material, from the cache or by opening it. An expired one signs nothing
// (ErrCertificateExpired), unless expired is allowed: it still verifies
// what it signed while it was valid.
func (s *Signer) active(ctx context.Context, org store.Organization, env model.Environment, expired bool) (signing.Material, error) {
	cert, sealed, err := s.load(ctx, org.ID, env)
	if errors.Is(err, store.ErrNotFound) {
		return signing.Material{}, ErrNoActiveCertificate
	}
	if err != nil {
		return signing.Material{}, err
	}
	if !expired && cert.NotAfter != nil && s.now().After(*cert.NotAfter) {
		return signing.Material{}, ErrCertificateExpired
	}
	if m, ok := s.material.Get(cert.ID); ok {
		return m, nil
	}
	pem, err := s.keyring.Open(sealed, []byte(cert.ID))
	if err != nil {
		// Never the material: only which certificate.
		return signing.Material{}, fmt.Errorf("open the material of certificate %s: %w", cert.ID, err)
	}
	m, err := signing.DecodeMaterial(pem)
	clear(pem)
	if err != nil {
		return signing.Material{}, fmt.Errorf("decode the material of certificate %s: %w", cert.ID, err)
	}
	s.material.Put(cert.ID, m)
	return m, nil
}

// Sign signs body with the organization's active certificate in env.
func (s *Signer) Sign(ctx context.Context, org store.Organization, env model.Environment, body []byte, contentType string) (signing.Signed, error) {
	country, err := s.Country(org.Country)
	if err != nil {
		return signing.Signed{}, err
	}
	m, err := s.active(ctx, org, env, false)
	if err != nil {
		return signing.Signed{}, err
	}
	return country.Sign(m, body, contentType, signing.Options{})
}

// Verify checks a signature against the organization's active certificate
// in env, also an expired one.
func (s *Signer) Verify(ctx context.Context, org store.Organization, env model.Environment, signature []byte) (signing.Verification, error) {
	country, err := s.Country(org.Country)
	if err != nil {
		return signing.Verification{}, err
	}
	m, err := s.active(ctx, org, env, true)
	if err != nil {
		return signing.Verification{}, err
	}
	return country.Verify(m, signature)
}

// Forget drops a certificate's material from memory.
func (s *Signer) Forget(certificateID string) { s.material.Delete(certificateID) }
