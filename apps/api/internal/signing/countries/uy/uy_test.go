package uy

import (
	"fmt"
	"testing"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing/signingtest"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

func TestVectors(t *testing.T) {
	signingtest.Run(t, New(), "uy", signingtest.Expect{
		Tamper: [2]string{`<MntTotal>1180`, `<MntTotal>1`},
		Fixed: map[string]string{
			"root-ds": "xmldom serialized the Signature as ds:Signature after signing it unprefixed",
		},
		Helium: heliumPerCFE,
	})
}

// heliumPerCFE verifies each CFE on its own: URI="" means its CFE here,
// while a stock verifier reads it as the whole document. Each CFE becomes a
// document of its own, with the namespaces it inherits declared on it (the
// same inclusive canonical form).
func heliumPerCFE(t *testing.T, _ string, signed []byte) error {
	doc, err := xmldsig.Parse(signed)
	if err != nil {
		return err
	}
	targets, err := cfes(doc.Root())
	if err != nil {
		return err
	}
	for i, cfe := range targets {
		cp := cfe.Copy()
		for p := cfe.Parent(); p != nil; p = p.Parent() {
			for _, a := range p.Attr {
				if a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns") {
					if cp.SelectAttr(a.FullKey()) == nil {
						cp.CreateAttr(a.FullKey(), a.Value)
					}
				}
			}
		}
		one := etree.NewDocument()
		one.SetRoot(cp)
		if _, err := signingtest.Helium(xmldsig.Serialize(one), signingtest.Certificate(t)); err != nil {
			return fmt.Errorf("CFE %d: %w", i, err)
		}
	}
	return nil
}
