package timing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/internal/idgen"
)

// valueEvent is a value-type event (like ticking.Event).
type valueEvent struct {
	EventBase
	Payload int `json:"payload"`
}

// pointerEvent is used as a pointer-type event.
type pointerEvent struct {
	EventBase
	Tag string `json:"tag"`
}

func TestEventRegistryValueRoundTrip(t *testing.T) {
	RegisterEvent(valueEvent{})

	e := valueEvent{Payload: 42}
	e.At = 10
	e.Handler = "h"

	if err := eventCodec.CheckRoundTrip(e); err != nil {
		t.Fatalf("CheckRoundTrip (value): %v", err)
	}
}

func TestEventRegistryPointerRoundTrip(t *testing.T) {
	RegisterEvent(&pointerEvent{})

	e := &pointerEvent{Tag: "x"}
	e.At = 20

	if err := eventCodec.CheckRoundTrip(e); err != nil {
		t.Fatalf("CheckRoundTrip (pointer): %v", err)
	}
}

func TestEventRegistryUnknownType(t *testing.T) {
	_, err := eventCodec.DecodeSlice(
		json.RawMessage(`[{"type":"nope.Type","payload":{}}]`))
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}

// TestEventBaseRegisteredByDefault confirms a plain event scheduled via
// MakeEventBase round-trips without the caller registering EventBase: the timing
// package registers its own built-in event type.
func TestEventBaseRegisteredByDefault(t *testing.T) {
	e := MakeEventBase((&idgen.IDGenerator{}).NewID(), 42, "h")
	e.ID = 7

	if err := eventCodec.CheckRoundTrip(e); err != nil {
		t.Fatalf("CheckRoundTrip (is EventBase registered by default?): %v", err)
	}
}

func TestEventBaseJSON(t *testing.T) {
	const wantJSON = `{"id":7,"time":42,"handler_id":"h","secondary":true}`
	var event EventBase
	if err := json.Unmarshal([]byte(wantJSON), &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != 7 || event.Time() != 42 || event.HandlerID() != "h" || !event.IsSecondary() {
		t.Fatalf("unexpected event fields: %+v", event)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != wantJSON {
		t.Fatalf("event JSON = %s, want %s", encoded, wantJSON)
	}
}
