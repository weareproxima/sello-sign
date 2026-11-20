package bo

import (
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "bo", signingtest.Expect{
		Tamper: [2]string{`<cuf>8727F63A`, `<cuf>0727F63A`},
		Fixed: map[string]string{
			"root-xsi": "SignedInfo was canonicalized without the xmlns:xsi it inherits",
			"edge":     "the comment counted in the digest, but URI=\"\" drops comments",
		},
	})
}
