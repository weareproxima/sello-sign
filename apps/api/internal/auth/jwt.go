package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// SelloWorkspace is one of the person's organizations at Sello's IdP, as
// the panel's token carries them (`sello_workspaces`) in the cloud.
type SelloWorkspace struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	Role    string `json:"role"`
}

// Claims are what the API reads from the panel's token.
type Claims struct {
	Subject   string
	Email     string
	Name      string
	ExpiresAt time.Time
	// SelloSub is the `sub` of the Sello account linked to the panel's user
	// (cloud only); empty otherwise.
	SelloSub string
	// SelloWorkspaces are the person's organizations at Sello's IdP, as its
	// ID token said when they last signed in (cloud only).
	SelloWorkspaces []SelloWorkspace
}

var (
	// ErrInvalidToken is any way a token fails but expiry: malformed,
	// another algorithm, unknown key, bad signature, wrong issuer or
	// audience.
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpiredToken: the token was valid and no longer is; the panel asks
	// for another.
	ErrExpiredToken = errors.New("expired token")
	// ErrKeysUnavailable: the panel's keys couldn't be fetched and the
	// token's `kid` isn't among those known. Always wrapped together with
	// ErrInvalidToken.
	ErrKeysUnavailable = errors.New("panel keys unavailable")
)

// refusal is ErrInvalidToken with the check that failed, for the logs.
type refusal string

func (r refusal) Error() string        { return "invalid token: " + string(r) }
func (r refusal) Is(target error) bool { return target == ErrInvalidToken }

const (
	refusedMalformed refusal = "malformed"
	refusedSignature refusal = "signature"
	refusedIssuer    refusal = "issuer"
	refusedAudience  refusal = "audience"
	refusedType      refusal = "type"
)

// RefusalReason says, for a log line, why Verify refused a token: never
// anything of the token itself.
func RefusalReason(err error) string {
	var r refusal
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrExpiredToken):
		return "expired"
	case errors.Is(err, ErrKeysUnavailable):
		return "keys"
	case errors.As(err, &r):
		return string(r)
	}
	return "invalid"
}

const (
	leeway         = 30 * time.Second
	jwksTTL        = time.Hour
	jwksMinRefresh = time.Minute
	jwksBackoff    = time.Second
	jwksMaxBackoff = time.Minute
)

// TokenVerifier checks a panel token.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (Claims, error)
}

// JWKSVerifier verifies the panel's tokens with the keys it publishes at
// <PANEL_URL>/api/auth/jwks: better-auth's jwt plugin, EdDSA over Ed25519.
// It's Sello's verifier of its IdP's tokens (pkg/httpkit/jwt.go there),
// without the OAuth access tokens: the keys are kept an hour; an unknown
// `kid` triggers one more fetch, at most once a minute; a failed fetch is
// retried after a second, doubling up to a minute, while the keys already
// known keep verifying. Only EdDSA is accepted, so no algorithm downgrade.
type JWKSVerifier struct {
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client
	now      func() time.Time
	flight   singleflight.Group

	mu        sync.Mutex
	keys      map[string]ed25519.PublicKey
	fetched   time.Time
	attempted time.Time
	failures  int
	lastErr   error
}

// NewJWKSVerifier verifies tokens of issuer (PANEL_URL) for audience
// (API_URL), with the keys at issuer's /api/auth/jwks.
func NewJWKSVerifier(issuer, audience string) *JWKSVerifier {
	return NewJWKSVerifierFrom(issuer, audience, strings.TrimRight(issuer, "/")+"/api/auth/jwks")
}

// NewJWKSVerifierFrom is NewJWKSVerifier with the keys somewhere else.
func NewJWKSVerifierFrom(issuer, audience, jwksURL string) *JWKSVerifier {
	return &JWKSVerifier{
		issuer: issuer, audience: audience, jwksURL: jwksURL,
		client: &http.Client{Timeout: 5 * time.Second},
		now:    time.Now,
		keys:   map[string]ed25519.PublicKey{},
	}
}

// WithClock makes the verifier read the time from now (tests).
func (v *JWKSVerifier) WithClock(now func() time.Time) *JWKSVerifier {
	v.now = now
	return v
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtPayload struct {
	Iss             string           `json:"iss"`
	Sub             string           `json:"sub"`
	Aud             json.RawMessage  `json:"aud"`
	Exp             float64          `json:"exp"`
	Email           string           `json:"email"`
	Name            string           `json:"name"`
	SelloSub        string           `json:"sello_sub"`
	SelloWorkspaces []SelloWorkspace `json:"sello_workspaces"`
}

// Verify checks the algorithm, the signature, the issuer, the audience and
// the expiry, in that order.
func (v *JWKSVerifier) Verify(ctx context.Context, token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, refusedMalformed
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, refusedMalformed
	}
	var h jwtHeader
	if err := json.Unmarshal(rawHeader, &h); err != nil || h.Alg != "EdDSA" || h.Kid == "" {
		return Claims{}, refusedMalformed
	}
	// An OAuth access token isn't the panel's: refused.
	if strings.EqualFold(h.Typ, "at+jwt") || strings.EqualFold(h.Typ, "application/at+jwt") {
		return Claims{}, refusedType
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, refusedMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Claims{}, refusedMalformed
	}
	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return Claims{}, err
	}
	if !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return Claims{}, refusedSignature
	}
	var p jwtPayload
	if err := json.Unmarshal(rawPayload, &p); err != nil {
		return Claims{}, refusedMalformed
	}
	if p.Iss != v.issuer {
		return Claims{}, refusedIssuer
	}
	if p.Sub == "" || p.Exp <= 0 {
		return Claims{}, refusedMalformed
	}
	if !hasAudience(p.Aud, v.audience) {
		return Claims{}, refusedAudience
	}
	exp := time.Unix(int64(p.Exp), 0)
	if v.now().After(exp.Add(leeway)) {
		return Claims{}, ErrExpiredToken
	}
	return Claims{
		Subject: p.Sub, Email: p.Email, Name: p.Name, ExpiresAt: exp,
		SelloSub: p.SelloSub, SelloWorkspaces: p.SelloWorkspaces,
	}, nil
}

func hasAudience(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}

func keysUnavailable(cause error) error {
	return fmt.Errorf("%w: %w: %w", ErrInvalidToken, ErrKeysUnavailable, cause)
}

func (v *JWKSVerifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	v.mu.Lock()
	now := v.now()
	k, known := v.keys[kid]
	if known && !v.fetched.IsZero() && now.Sub(v.fetched) < jwksTTL {
		v.mu.Unlock()
		return k, nil
	}
	fetch, failed := v.mayFetch(now), v.lastErr
	v.mu.Unlock()
	if !fetch {
		switch {
		case known:
			return k, nil
		case failed != nil:
			return nil, keysUnavailable(failed)
		}
		return nil, refusedSignature
	}
	done := v.flight.DoChan("jwks", func() (any, error) {
		return nil, v.refresh(context.WithoutCancel(ctx))
	})
	var err error
	select {
	case res := <-done:
		err = res.Err
	case <-ctx.Done():
		err = ctx.Err()
	}
	v.mu.Lock()
	k, known = v.keys[kid]
	v.mu.Unlock()
	switch {
	case known:
		return k, nil
	case err != nil:
		return nil, keysUnavailable(err)
	}
	return nil, refusedSignature
}

func (v *JWKSVerifier) mayFetch(now time.Time) bool {
	if v.failures > 0 {
		return now.Sub(v.attempted) >= backoff(v.failures)
	}
	return v.fetched.IsZero() || now.Sub(v.fetched) >= jwksMinRefresh
}

func backoff(failures int) time.Duration {
	d := jwksBackoff
	for i := 1; i < failures && d < jwksMaxBackoff; i++ {
		d *= 2
	}
	return min(d, jwksMaxBackoff)
}

func (v *JWKSVerifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	if !v.mayFetch(v.now()) {
		err := v.lastErr
		v.mu.Unlock()
		return err
	}
	v.mu.Unlock()
	keys, err := v.download(ctx)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.attempted = v.now()
	if err != nil {
		v.failures++
		v.lastErr = err
		return err
	}
	v.keys, v.fetched, v.failures, v.lastErr = keys, v.attempted, 0, nil
	return nil
}

type jwks struct {
	Keys []struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		Kid string `json:"kid"`
		X   string `json:"x"`
	} `json:"keys"`
}

func (v *JWKSVerifier) download(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", v.jwksURL, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var set jwks
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, err
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(x)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: no Ed25519 keys", v.jwksURL)
	}
	return keys, nil
}
