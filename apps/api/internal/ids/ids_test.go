package ids

import (
	"testing"
	"time"
)

func TestFloor(t *testing.T) {
	base := time.Now().Add(time.Hour)
	before := NewAt(base).String()
	floor := Floor(base.Add(time.Millisecond))
	after := NewAt(base.Add(2 * time.Millisecond)).String()
	if !(before < floor && floor < after) {
		t.Fatalf("%s < %s < %s doesn't hold", before, floor, after)
	}
	if !Valid(floor) {
		t.Fatalf("%s isn't a UUID", floor)
	}
}
