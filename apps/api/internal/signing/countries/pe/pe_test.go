package pe

import (
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "pe", signingtest.Expect{
		Tamper: [2]string{`PayableAmount currencyID="PEN">118.00`, `PayableAmount currencyID="PEN">1.00`},
		Fixed: map[string]string{
			"no-ext-prefix": "SignedInfo was canonicalized without the xmlns:ext xmldom added later",
		},
	})
}
