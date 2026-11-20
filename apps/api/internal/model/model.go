// Package model is Sello Sign's vocabulary and its rules, without I/O: the
// environments, the roles of a person in a space, the scopes of a key and of
// a delegation, and who reaches which organization.
//
// A space (workspace) is Sign's customer and keeps organizations (the
// taxpayers); a delegation lets another space sign with an organization, and
// maybe change its certificates.
package model

import (
	"slices"
	"strings"
	"time"
	"unicode"
)

// Environment is where a signature is meant to go: the authority's test
// environment or production.
type Environment string

const (
	Test Environment = "test"
	Prod Environment = "prod"
)

// ParseEnvironment reads "test" or "prod".
func ParseEnvironment(s string) (Environment, bool) {
	switch Environment(s) {
	case Test, Prod:
		return Environment(s), true
	}
	return "", false
}

// Role is a person's in a space.
type Role string

const (
	Owner  Role = "owner"
	Admin  Role = "admin"
	Member Role = "member"
)

// ParseRole reads a role in any case (billing writes them in upper case).
func ParseRole(s string) (Role, bool) {
	switch r := Role(strings.ToLower(s)); r {
	case Owner, Admin, Member:
		return r, true
	}
	return "", false
}

// rank orders the roles: owner > admin > member.
func (r Role) rank() int {
	switch r {
	case Owner:
		return 3
	case Admin:
		return 2
	case Member:
		return 1
	}
	return 0
}

// Higher returns the higher of two roles; "" counts as none.
func Higher(a, b Role) Role {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Manages says whether the role administers the space: owner or admin.
func (r Role) Manages() bool { return r == Owner || r == Admin }

// Scope is something a key may do.
type Scope string

const (
	ScopeSign               Scope = "sign"
	ScopeOrganizationsRead  Scope = "organizations:read"
	ScopeOrganizationsWrite Scope = "organizations:write"
	ScopeCertificatesRead   Scope = "certificates:read"
	ScopeCertificatesWrite  Scope = "certificates:write"
	ScopeDelegationsRead    Scope = "delegations:read"
	ScopeDelegationsWrite   Scope = "delegations:write"
	ScopeKeysRead           Scope = "keys:read"
	ScopeKeysWrite          Scope = "keys:write"
	ScopeLogsRead           Scope = "logs:read"
)

// AllScopes is every scope, in the contract's order.
var AllScopes = []Scope{
	ScopeSign, ScopeOrganizationsRead, ScopeOrganizationsWrite, ScopeCertificatesRead, ScopeCertificatesWrite,
	ScopeDelegationsRead, ScopeDelegationsWrite, ScopeKeysRead, ScopeKeysWrite, ScopeLogsRead,
}

// DelegationScopes are what a delegation may grant.
var DelegationScopes = []Scope{ScopeSign, ScopeCertificatesRead, ScopeCertificatesWrite}

// PlatformDelegationScopes are what Sello's platform keeps when a company
// takes its organizations to its own space: sign and change the
// certificates.
var PlatformDelegationScopes = DelegationScopes

// memberScopes are what a member's role allows: reading. Signing from the
// panel in test is allowed apart (CanSignFromPanel).
var memberScopes = []Scope{ScopeOrganizationsRead, ScopeCertificatesRead, ScopeDelegationsRead, ScopeKeysRead, ScopeLogsRead}

// ScopesOfRole is what a person may do in a space by their role.
func ScopesOfRole(r Role) []Scope {
	switch r {
	case Owner, Admin:
		return AllScopes
	case Member:
		return memberScopes
	}
	return nil
}

// CanSignFromPanel says whether a role may sign from the panel in env:
// owners and admins in both, members only in test.
func CanSignFromPanel(r Role, env Environment) bool {
	return r.Manages() || (r == Member && env == Test)
}

// ParseScopes reads a list of scopes against allowed, without duplicates,
// in allowed's order; ok is false if any isn't allowed or the list is empty.
func ParseScopes(in []string, allowed []Scope) ([]Scope, bool) {
	if len(in) == 0 {
		return nil, false
	}
	set := map[Scope]bool{}
	for _, s := range in {
		sc := Scope(s)
		if !slices.Contains(allowed, sc) {
			return nil, false
		}
		set[sc] = true
	}
	out := make([]Scope, 0, len(set))
	for _, s := range allowed {
		if set[s] {
			out = append(out, s)
		}
	}
	return out, true
}

// HasScope says whether scopes holds s.
func HasScope(scopes []Scope, s Scope) bool { return slices.Contains(scopes, s) }

// Subset says whether every scope of a is in b.
func Subset(a, b []Scope) bool {
	for _, s := range a {
		if !slices.Contains(b, s) {
			return false
		}
	}
	return true
}

// Strings is scopes as text, for the database and JSON.
func Strings(scopes []Scope) []string {
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

// ScopesOf reads scopes the database wrote, keeping only known ones.
func ScopesOf(in []string) []Scope {
	out := make([]Scope, 0, len(in))
	for _, s := range in {
		if slices.Contains(AllScopes, Scope(s)) {
			out = append(out, Scope(s))
		}
	}
	return out
}

// TaxIDKey is a tax id as it's compared: without separators (dashes,
// spaces, dots, slashes), in upper case. "0614-123456-101-2" and
// "06141234561012" are the same taxpayer.
func TaxIDKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

// ValidCountry says whether s looks like an ISO 3166-1 alpha-2 code in
// upper case.
func ValidCountry(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// Month is a month's key as the usage counters and billing write it:
// "2026-11", in UTC.
func Month(t time.Time) string { return t.UTC().Format("2006-01") }

// MonthsToReport are the months the hourly usage report sends at now: the
// current one and, during its first three days, the one before, to settle it
// with what was counted late.
func MonthsToReport(now time.Time) []string {
	now = now.UTC()
	months := []string{Month(now)}
	if now.Day() <= 3 {
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		months = append(months, Month(first.AddDate(0, -1, 0)))
	}
	return months
}

// Relation is how a space holds an organization.
type Relation string

const (
	Owned     Relation = "owned"
	Delegated Relation = "delegated"
)

// Reach is what a space may do with an organization it holds: everything
// if it owns it; what the delegation says if it's delegated to it.
type Reach struct {
	Relation Relation
	// Scopes are the delegation's; nil when the space owns it.
	Scopes []Scope
}

// Allows says whether the reach lets the space do s with the organization:
// an owner, anything; a delegate, only its delegation's scopes.
func (r Reach) Allows(s Scope) bool {
	if r.Relation == Owned {
		return true
	}
	return r.Relation == Delegated && HasScope(r.Scopes, s)
}

// CanInvite says whether a person with role actor may invite someone as
// role: owners and admins invite, and only an owner invites an owner.
func CanInvite(actor, role Role) bool {
	return actor.Manages() && (role != Owner || actor == Owner)
}

// CanSetRole says whether a person with role actor may change a member's
// role from from to to: owners and admins change roles, and only an owner
// makes or unmakes an owner. That the space keeps an owner is the store's
// to check, under a lock.
func CanSetRole(actor, from, to Role) bool {
	if !actor.Manages() {
		return false
	}
	if from == Owner || to == Owner {
		return actor == Owner
	}
	return true
}

// NormalizeEmail is an e-mail address as it's compared: trimmed and in
// lower case. Mail servers don't care about case.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ValidEmail says whether s, normalized, looks like an address: one @ with
// something on both sides, a dot in the domain, no spaces, at most 320
// characters. The e-mail itself is the real check.
func ValidEmail(s string) bool {
	if len(s) > 320 || strings.ContainsAny(s, " \t\r\n<>,;\"") {
		return false
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || strings.Contains(domain, "@") {
		return false
	}
	dot := strings.LastIndex(domain, ".")
	return dot > 0 && dot < len(domain)-1
}

// CanRemove says whether a person with role actor may take a member with
// role target out of a space: anyone leaves by themselves (self); owners
// and admins remove others, and only an owner removes an owner. That the
// space keeps an owner is the store's to check, under a lock.
func CanRemove(actor, target Role, self bool) bool {
	if self {
		return actor != ""
	}
	if !actor.Manages() {
		return false
	}
	return target != Owner || actor == Owner
}
