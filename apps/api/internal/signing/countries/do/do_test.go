package do

import (
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "do", signingtest.Expect{
		Tamper: [2]string{`<MontoTotal>1180.00`, `<MontoTotal>1.00`},
		Fixed: map[string]string{
			"root-ds":    "xmldom serialized the Signature as ds:Signature after signing it unprefixed",
			"cr-in-text": "xmldom wrote the carriage return raw, which a verifier reads as a line feed",
			"doc-pi":     "URI=\"\" covers the processing instruction before the root, which TS left out of the digest",
		},
	})
}
