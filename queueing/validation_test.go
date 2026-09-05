package queueing

import (
	"encoding/json"
	"testing"
)

func TestBufferRejectsInvalidJSONBeforeAssignment(t *testing.T) {
	for _, input := range []string{
		`{"cap":-1}`, `{"cap":1,"elements":[1,2]}`,
	} {
		b := NewBuffer[int]("original", 2)
		b.PushTyped(9)
		if err := json.Unmarshal([]byte(input), &b); err == nil {
			t.Fatalf("accepted %s", input)
		}
		if b.Name() != "original" || b.Capacity() != 2 || b.Peek() != 9 {
			t.Fatal("invalid JSON mutated target")
		}
	}
}

func TestPipelineRejectsInvalidGeometryAndOccupancy(t *testing.T) {
	for _, input := range []string{
		`{"width":-1}`, `{"num_stages":-1}`,
		`{"width":1,"num_stages":1,"stages":[{"lane":1}]}`,
		`{"width":1,"num_stages":1,"stages":[{"stage":1}]}`,
		`{"width":1,"num_stages":1,"stages":[{"cycle_left":-1}]}`,
		`{"width":1,"num_stages":1,"stages":[{},{}]}`,
	} {
		var p Pipeline[int]
		if err := json.Unmarshal([]byte(input), &p); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
