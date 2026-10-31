package lruset

import "testing"

// TestUpdateKeyKeepsKeyRefilledElsewhere covers a block that is invalidated in
// place (its key still points to its way) and then refilled into another way.
// Reusing the invalidated way later must not unmap the refilled key.
func TestUpdateKeyKeepsKeyRefilledElsewhere(t *testing.T) {
	s := MakeSet(4)
	key := KeyString(1, 0x1000)

	s.UpdateKey(0, "", key) // filled into way 0, then invalidated in place
	s.UpdateKey(2, "", key) // refilled into way 2

	if w, ok := s.Lookup(key); !ok || w != 2 {
		t.Fatalf("Lookup after refill = %d, %v; want 2, true", w, ok)
	}

	s.UpdateKey(0, key, KeyString(1, 0x4000)) // way 0 reused

	if w, ok := s.Lookup(key); !ok || w != 2 {
		t.Fatalf("Lookup after reusing the stale way = %d, %v; want 2, true",
			w, ok)
	}

	if w, ok := s.Lookup(KeyString(1, 0x4000)); !ok || w != 0 {
		t.Fatalf("Lookup of the new key = %d, %v; want 0, true", w, ok)
	}
}
