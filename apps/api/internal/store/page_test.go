package store

import (
	"testing"

	"proxima.la/sello/sign/internal/ids"
)

func TestCursor(t *testing.T) {
	id := ids.New()
	c := EncodeCursor(id)
	got, ok := DecodeCursor(c)
	if !ok || got != id {
		t.Fatalf("DecodeCursor = %q %v", got, ok)
	}
	if _, ok := DecodeCursor("!!"); ok {
		t.Error("accepted garbage")
	}
	if _, ok := DecodeCursor(EncodeCursor("not-an-id")); ok {
		t.Error("accepted a cursor that isn't an id")
	}
	if got, ok := DecodeCursor(""); !ok || got != "" {
		t.Error("the first page")
	}
}

func TestTrim(t *testing.T) {
	items := []string{"a", "b", "c"}
	got, next := trim(items, Page{Limit: 2}, func(s string) string { return s })
	if len(got) != 2 || next == nil || *next != EncodeCursor("b") {
		t.Fatalf("trim = %v %v", got, next)
	}
	got, next = trim(items[:2], Page{Limit: 2}, func(s string) string { return s })
	if len(got) != 2 || next != nil {
		t.Fatalf("last page = %v %v", got, next)
	}
	if limitArg(Page{Limit: 500}) != MaxLimit+1 || limitArg(Page{}) != DefaultLimit+1 {
		t.Error("limitArg")
	}
}

func TestIDsAreOrdered(t *testing.T) {
	prev := ids.New()
	for range 10_000 {
		next := ids.New()
		if next <= prev {
			t.Fatalf("%s after %s", next, prev)
		}
		prev = next
	}
}
