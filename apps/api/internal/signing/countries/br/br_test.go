package br

import (
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "br", signingtest.Expect{
		Tamper: [2]string{`<CNPJ>12345678000199`, `<CNPJ>12345678000198`},
		Fixed: map[string]string{
			"cr-in-text": "the carriage return is written &#13; instead of raw",
		},
		TSStillValid: map[string]string{
			"cr-in-text": "br.ts digested a re-parsed copy, so its signature holds, but the document says a line feed where the sender put a carriage return",
		},
	})
}
