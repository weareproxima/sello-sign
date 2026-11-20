// Package countries builds the registry of every country Sign signs for.
// Which ones the API offers is its configuration (SIGN_COUNTRIES).
package countries

import (
	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/countries/bo"
	"proxima.la/sello/sign/internal/signing/countries/br"
	"proxima.la/sello/sign/internal/signing/countries/co"
	"proxima.la/sello/sign/internal/signing/countries/cr"
	"proxima.la/sello/sign/internal/signing/countries/do"
	"proxima.la/sello/sign/internal/signing/countries/ec"
	"proxima.la/sello/sign/internal/signing/countries/gt"
	"proxima.la/sello/sign/internal/signing/countries/pa"
	"proxima.la/sello/sign/internal/signing/countries/pe"
	"proxima.la/sello/sign/internal/signing/countries/sv"
	"proxima.la/sello/sign/internal/signing/countries/uy"
)

// Registry holds all of them.
func Registry() *signing.Registry {
	return signing.NewRegistry(
		bo.New(), br.New(), co.New(), cr.New(), do.New(), ec.New(),
		gt.New(), pa.New(), pe.New(), sv.New(), uy.New(),
	)
}
