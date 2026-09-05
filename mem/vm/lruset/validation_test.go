package lruset

import (
	"encoding/json"
	"testing"
)

func TestRestoreRejectsInvalidLRUState(t *testing.T) {
	for _, input := range []string{
		`{"way_count":-1}`,
		`{"way_count":1,"last_visits":[]}`,
		`{"way_count":1,"last_visits":[0],"visit_list":[1]}`,
		`{"way_count":1,"last_visits":[0],"visit_list":[0,0]}`,
		`{"way_count":1,"last_visits":[0],"key_map":{"key":-1}}`,
		`{"way_count":1,"last_visits":[2],"visit_count":1}`,
	} {
		s := NewSet(2)
		if err := json.Unmarshal([]byte(input), &s); err == nil {
			t.Fatalf("accepted %s", input)
		}
		if way, ok := s.Evict(); !ok || way != 0 {
			t.Fatal("invalid JSON changed the original eviction order")
		}
	}
}
