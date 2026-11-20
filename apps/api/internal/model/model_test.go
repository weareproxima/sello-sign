package model

import (
	"testing"
)

func TestReach(t *testing.T) {
	owned := Reach{Relation: Owned}
	for _, s := range AllScopes {
		if !owned.Allows(s) {
			t.Errorf("the owner can't %s", s)
		}
	}
	signOnly := Reach{Relation: Delegated, Scopes: []Scope{ScopeSign}}
	if !signOnly.Allows(ScopeSign) || signOnly.Allows(ScopeCertificatesWrite) || signOnly.Allows(ScopeCertificatesRead) {
		t.Error("a sign-only delegation allows the wrong things")
	}
	if (Reach{}).Allows(ScopeSign) {
		t.Error("no reach allows signing")
	}
}

func TestRoles(t *testing.T) {
	if !Subset(AllScopes, ScopesOfRole(Owner)) || !Subset(AllScopes, ScopesOfRole(Admin)) {
		t.Error("owners and admins don't have every scope")
	}
	m := ScopesOfRole(Member)
	for _, s := range []Scope{ScopeSign, ScopeOrganizationsWrite, ScopeCertificatesWrite, ScopeDelegationsWrite, ScopeKeysWrite} {
		if HasScope(m, s) {
			t.Errorf("a member has %s", s)
		}
	}
	if !CanSignFromPanel(Member, Test) || CanSignFromPanel(Member, Prod) || !CanSignFromPanel(Admin, Prod) || CanSignFromPanel("", Test) {
		t.Error("signing from the panel by role is wrong")
	}
	if Higher(Member, Owner) != Owner || Higher(Admin, "") != Admin {
		t.Error("Higher")
	}
	if r, ok := ParseRole("OWNER"); !ok || r != Owner {
		t.Error("ParseRole upper case")
	}
}

func TestParseScopes(t *testing.T) {
	got, ok := ParseScopes([]string{"certificates:write", "sign", "sign"}, DelegationScopes)
	if !ok || len(got) != 2 || got[0] != ScopeSign || got[1] != ScopeCertificatesWrite {
		t.Fatalf("got %v %v", got, ok)
	}
	if _, ok := ParseScopes([]string{"keys:write"}, DelegationScopes); ok {
		t.Error("a delegation took keys:write")
	}
	if _, ok := ParseScopes(nil, AllScopes); ok {
		t.Error("an empty list passed")
	}
}

func TestTaxIDKey(t *testing.T) {
	for in, want := range map[string]string{
		"0614-123456-101-2": "06141234561012",
		" 3-101-123456 ":    "3101123456",
		"abc.123/4":         "ABC1234",
	} {
		if got := TaxIDKey(in); got != want {
			t.Errorf("TaxIDKey(%q) = %q, want %q", in, got, want)
		}
	}
}
