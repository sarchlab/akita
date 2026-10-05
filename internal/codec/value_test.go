package codec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSingleValueRoundTrip(t *testing.T) {
	type value struct{ Data []int }
	r := NewRegistry[any]("test")
	r.Register(value{})
	for _, want := range []any{nil, value{Data: []int{1, 2}}} {
		raw, err := r.Encode(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
	if _, err := r.Encode(&value{}); err == nil {
		t.Fatal("accepted unregistered pointer")
	}
	for _, raw := range []string{`{"payload":{}}`, `{"type":"unknown","payload":{}}`, `{"type":"unknown"}`, `[]`} {
		if _, err := r.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
