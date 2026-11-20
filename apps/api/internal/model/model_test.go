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

func TestCanInvite(t *testing.T) {
	for _, c := range []struct {
		actor, role Role
		want        bool
	}{
		{Owner, Owner, true}, {Owner, Admin, true}, {Owner, Member, true},
		{Admin, Owner, false}, {Admin, Admin, true}, {Admin, Member, true},
		{Member, Member, false}, {"", Member, false},
	} {
		if got := CanInvite(c.actor, c.role); got != c.want {
			t.Errorf("CanInvite(%s, %s) = %v", c.actor, c.role, got)
		}
	}
}

func TestCanSetRole(t *testing.T) {
	for _, c := range []struct {
		actor, from, to Role
		want            bool
	}{
		{Owner, Member, Owner, true},
		{Owner, Owner, Admin, true},
		{Admin, Member, Admin, true},
		{Admin, Admin, Member, true},
		{Admin, Member, Owner, false},
		{Admin, Owner, Admin, false},
		{Member, Member, Admin, false},
	} {
		if got := CanSetRole(c.actor, c.from, c.to); got != c.want {
			t.Errorf("CanSetRole(%s, %s → %s) = %v", c.actor, c.from, c.to, got)
		}
	}
}

func TestEmail(t *testing.T) {
	if got := NormalizeEmail("  Ana@Example.COM "); got != "ana@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
	for in, want := range map[string]bool{
		"ana@example.com": true, "a.b+c@sub.example.co": true,
		"ana": false, "@example.com": false, "ana@": false, "ana@example": false,
		"ana@example.": false, "a b@example.com": false, "a@b@example.com": false,
		"Ana <ana@example.com>": false,
	} {
		if got := ValidEmail(in); got != want {
			t.Errorf("ValidEmail(%q) = %v", in, got)
		}
	}
}

func TestCanRemove(t *testing.T) {
	cases := []struct {
		actor, target Role
		self, want    bool
	}{
		{Member, Member, true, true},
		{Owner, Owner, true, true},
		{"", Member, true, false},
		{Member, Member, false, false},
		{Admin, Member, false, true},
		{Admin, Admin, false, true},
		{Admin, Owner, false, false},
		{Owner, Owner, false, true},
		{Owner, Admin, false, true},
	}
	for _, c := range cases {
		if got := CanRemove(c.actor, c.target, c.self); got != c.want {
			t.Errorf("CanRemove(%s, %s, self %v) = %v", c.actor, c.target, c.self, got)
		}
	}
}
