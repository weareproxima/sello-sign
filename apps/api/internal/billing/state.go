// Package billing is how Sello Sign's cloud talks to Sello's billing server
// (cmd/billing in Sello's monorepo), which sells each space's plan:
//
//   - billing → Sign (BILLING_PUSH_TOKEN): at every change of a space, its
//     whole state, numbered by a version, to
//     PUT /api/billing/organizations/{id}; Sign applies it if the version is
//     greater than the one it has (State, ParseState).
//   - Sign → billing (BILLING_TOKEN): activating a space when someone opens
//     it for the first time, which answers its state (Client.Activate), and
//     reporting every hour what each space signed in the month
//     (Client.ReportUsage, Reporter).
//
// The bodies are those of ../sign-old/src/lib/server/billing.ts and of
// billing_http.SignStateBody in Sello: Sello only changes the origin.
package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/model"
)

// Plans are the cloud's plans, as stored (lower case).
var Plans = []string{"free", "pro_starter", "pro_growth", "pro_scale", "pro_business", "enterprise"}

// Member is a seat billing pushes: the `sub` of a Sello account and its
// role.
type Member struct {
	Account string
	Role    model.Role
}

// Limits are a plan's; nil is none.
type Limits struct {
	Issuers           *int
	ProdSignatures    *int
	TestSignatures    *int
	Members           *int
	RequestsPerMinute *int
	RetentionDays     *int
}

// Subscription is what Sign is told of a space's subscription.
type Subscription struct {
	Status            string
	Interval          *string
	CurrentPeriodEnd  *time.Time
	CancelAtPeriodEnd bool
}

// State is a space as billing pushes it.
type State struct {
	// WorkspaceID is the organization's id at Sello's IdP.
	WorkspaceID string
	Version     int64
	// Closed: Sello deleted the organization.
	Closed  bool
	Name    string
	Country *string
	// Members are in seat order: the owner, then by when they joined.
	Members      []Member
	Plan         string
	Limits       Limits
	ProdHardCap  *int
	TestHardCap  *int
	Subscription *Subscription
}

// Applied is what became of a pushed state.
type Applied string

const (
	// AppliedNew: the state was applied.
	AppliedNew Applied = "applied"
	// AppliedStale: Sign already had that version or a greater one.
	AppliedStale Applied = "stale"
	// AppliedUnknown: Sign has no space of that organization.
	AppliedUnknown Applied = "unknown"
)

// ErrInvalidState: the body isn't a state.
var ErrInvalidState = errors.New("billing: not a state")

type rawState struct {
	WorkspaceID  *string                    `json:"workspace_id"`
	Version      *json.Number               `json:"version"`
	Closed       bool                       `json:"closed"`
	Name         *string                    `json:"name"`
	Country      *string                    `json:"country"`
	Members      []rawMember                `json:"members"`
	Plan         *string                    `json:"plan"`
	Limits       map[string]json.RawMessage `json:"limits"`
	HardCaps     map[string]json.RawMessage `json:"hard_caps"`
	Subscription *rawSubscription           `json:"subscription"`
}

type rawMember struct {
	Account string `json:"account"`
	Role    string `json:"role"`
}

type rawSubscription struct {
	Status            *string `json:"status"`
	Interval          *string `json:"interval"`
	CurrentPeriodEnd  *string `json:"current_period_end"`
	CancelAtPeriodEnd bool    `json:"cancel_at_period_end"`
}

// limitOf reads a limit: a whole number ≥ 0, or null for none. ok is false
// for anything else, also when the key is missing and required.
func limitOf(m map[string]json.RawMessage, key string, required bool) (*int, bool) {
	raw, present := m[key]
	if !present {
		return nil, !required
	}
	if string(raw) == "null" {
		return nil, true
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, false
	}
	i, err := n.Int64()
	if err != nil || i < 0 || i > 1<<31-1 {
		return nil, false
	}
	v := int(i)
	return &v, true
}

// ParseState reads a state as the billing server writes it (snake_case,
// plans and roles in upper case). Every limit must be there, as a number
// or null.
func ParseState(body []byte) (State, error) {
	var r rawState
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return State{}, ErrInvalidState
	}
	if r.WorkspaceID == nil || *r.WorkspaceID == "" || r.Version == nil || r.Plan == nil || r.Limits == nil {
		return State{}, ErrInvalidState
	}
	version, err := r.Version.Int64()
	if err != nil {
		return State{}, ErrInvalidState
	}
	plan := strings.ToLower(*r.Plan)
	known := false
	for _, p := range Plans {
		known = known || p == plan
	}
	if !known {
		return State{}, ErrInvalidState
	}
	st := State{WorkspaceID: *r.WorkspaceID, Version: version, Closed: r.Closed, Plan: plan}
	if r.Name != nil {
		st.Name = *r.Name
	}
	if r.Country != nil && *r.Country != "" {
		c := strings.ToUpper(*r.Country)
		st.Country = &c
	}
	var ok bool
	l := &st.Limits
	for _, f := range []struct {
		key string
		dst **int
	}{
		{"issuers", &l.Issuers}, {"prod_signatures", &l.ProdSignatures}, {"test_signatures", &l.TestSignatures},
		{"members", &l.Members}, {"requests_per_minute", &l.RequestsPerMinute}, {"retention_days", &l.RetentionDays},
	} {
		if *f.dst, ok = limitOf(r.Limits, f.key, true); !ok {
			return State{}, ErrInvalidState
		}
	}
	if r.HardCaps != nil {
		if st.ProdHardCap, ok = limitOf(r.HardCaps, "prod", false); !ok {
			return State{}, ErrInvalidState
		}
		if st.TestHardCap, ok = limitOf(r.HardCaps, "test", false); !ok {
			return State{}, ErrInvalidState
		}
	}
	for _, m := range r.Members {
		role, ok := model.ParseRole(m.Role)
		if m.Account == "" || !ok {
			return State{}, ErrInvalidState
		}
		st.Members = append(st.Members, Member{Account: m.Account, Role: role})
	}
	if s := r.Subscription; s != nil {
		sub := &Subscription{Status: "active", CancelAtPeriodEnd: s.CancelAtPeriodEnd}
		if s.Status != nil {
			sub.Status = strings.ToLower(*s.Status)
		}
		if s.Interval != nil {
			if i := strings.ToLower(*s.Interval); i == "month" || i == "year" {
				sub.Interval = &i
			}
		}
		if s.CurrentPeriodEnd != nil {
			if t, err := time.Parse(time.RFC3339Nano, *s.CurrentPeriodEnd); err == nil {
				sub.CurrentPeriodEnd = &t
			}
		}
		st.Subscription = sub
	}
	return st, nil
}

// UsageEntry is one month of one space, as POST /billing/sign/usage takes
// it: the whole count so far.
type UsageEntry struct {
	WorkspaceID string `json:"workspace_id"`
	Period      string `json:"period"`
	Prod        int64  `json:"prod"`
	Test        int64  `json:"test"`
}
