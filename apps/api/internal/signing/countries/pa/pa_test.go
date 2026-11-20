package pa

import (
	"regexp"
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

// gNoFirm is outside the signature by the ficha's rule, not by a transform:
// a stock verifier sees the document without it.
var noFirm = regexp.MustCompile(`<gNoFirm>.*?</gNoFirm>`)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "pa", signingtest.Expect{
		Tamper: [2]string{`<dId>FE012`, `<dId>FE013`},
		Helium: func(t *testing.T, _ string, signed []byte) error {
			_, err := signingtest.Helium(noFirm.ReplaceAll(signed, nil), signingtest.Certificate(t))
			return err
		},
	})
}
