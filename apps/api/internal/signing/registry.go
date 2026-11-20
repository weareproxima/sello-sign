package signing

import (
	"fmt"
	"slices"
	"strings"
)

// Registry is the set of countries the API signs for.
type Registry struct {
	byCode map[string]Country
}

// NewRegistry builds a registry; two countries with one code is a bug.
func NewRegistry(countries ...Country) *Registry {
	r := &Registry{byCode: make(map[string]Country, len(countries))}
	for _, c := range countries {
		code := strings.ToUpper(c.Code())
		if _, dup := r.byCode[code]; dup {
			panic(fmt.Sprintf("signing: country %s registered twice", code))
		}
		r.byCode[code] = c
	}
	return r
}

// Lookup finds a country by its code, in any case.
func (r *Registry) Lookup(code string) (Country, bool) {
	c, ok := r.byCode[strings.ToUpper(code)]
	return c, ok
}

// Codes lists the registered codes, sorted.
func (r *Registry) Codes() []string {
	codes := make([]string, 0, len(r.byCode))
	for code := range r.byCode {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}
