package audit

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestActorFrom(t *testing.T) {
	if a := ActorFrom(context.Background()); a != System {
		t.Fatalf("without an actor = %+v", a)
	}
	key := Actor{Type: ActorAPIKey, ID: "k", Via: ViaMCP}
	if a := ActorFrom(WithActor(context.Background(), key)); a != key {
		t.Fatalf("actor = %+v", a)
	}
	if a := Space("w"); a.Type != ActorWorkspace || a.ID != "w" || a.Email != "" {
		t.Fatalf("space = %+v", a)
	}
}

func TestChanges(t *testing.T) {
	one, two := 1, 2
	name := "Ana"
	c := Changes{}
	c.Set("same", "x", "x")
	c.Set("same pointer", &one, &one)
	c.Set("nil to nil", (*int)(nil), (*int)(nil))
	c.Set("name", "Beto", &name)
	c.Set("rate", &one, &two)
	c.Set("cleared", &one, (*int)(nil))
	c.Set("set", (*string)(nil), "SV")
	if len(c) != 4 {
		t.Fatalf("changes = %v", c)
	}
	want := map[string]Change{
		"name": {From: "Beto", To: "Ana"}, "rate": {From: 1, To: 2},
		"cleared": {From: 1, To: nil}, "set": {From: nil, To: "SV"},
	}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("%s = %+v, want %+v", k, c[k], v)
		}
	}
}

// Every action names its target type before its dot, and is listed once.
func TestActions(t *testing.T) {
	seen := map[Action]bool{}
	for _, a := range Actions {
		target, _, ok := strings.Cut(string(a), ".")
		if !ok || !slices.Contains(TargetTypes, TargetType(target)) {
			t.Errorf("%s doesn't start with a target type", a)
		}
		if seen[a] {
			t.Errorf("%s twice", a)
		}
		seen[a] = true
	}
	if RetentionDays != 365 || Retention.Hours() != 365*24 {
		t.Fatalf("retention = %d days", RetentionDays)
	}
}
