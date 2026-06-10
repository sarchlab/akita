package lruset

import (
	"encoding/json"
	"testing"
)

// TestSetJSONRoundTrip locks in that a Set's full LRU state survives JSON
// serialization. Before MarshalJSON/UnmarshalJSON the unexported fields encoded
// as an empty object, so a restored set had an empty visit list and could not
// evict — which crashed a resumed TLB.
func TestSetJSONRoundTrip(t *testing.T) {
	s := NewSet(4)
	s.Visit(2)
	s.Visit(0)
	s.UpdateKey(2, "", KeyString(1, 0x1000))

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Set
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The key map round-trips.
	if w, ok := got.Lookup(KeyString(1, 0x1000)); !ok || w != 2 {
		t.Fatalf("Lookup after restore = %d, %v; want 2, true", w, ok)
	}

	// The eviction order is identical to the original's.
	for i := 0; i < 4; i++ {
		w1, ok1 := s.Evict()
		w2, ok2 := got.Evict()
		if ok1 != ok2 || w1 != w2 {
			t.Fatalf("evict %d: original (%d,%v) != restored (%d,%v)", i, w1, ok1, w2, ok2)
		}
	}

	// Both are exhausted identically — the restored set is not empty-from-birth.
	if _, ok := got.Evict(); ok {
		t.Fatalf("restored set should be exhausted after 4 evictions")
	}

	// A restored set accepts further key updates (keyMap is non-nil).
	got.UpdateKey(1, "", KeyString(2, 0x2000))
}
