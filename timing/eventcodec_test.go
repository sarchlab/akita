package timing

import (
	"encoding/json"
	"strings"
	"testing"
)

// valueEvent is a value-type event (like modeling.TickEvent).
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
	e.Time_ = 10
	e.HandlerID_ = "h"

	if err := CheckRoundTrip(e); err != nil {
		t.Fatalf("CheckRoundTrip (value): %v", err)
	}
}

func TestEventRegistryPointerRoundTrip(t *testing.T) {
	RegisterEvent(&pointerEvent{})

	e := &pointerEvent{Tag: "x"}
	e.Time_ = 20

	if err := CheckRoundTrip(e); err != nil {
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
	e := MakeEventBase(42, "h")
	e.ID = 7

	if err := CheckRoundTrip(e); err != nil {
		t.Fatalf("CheckRoundTrip (is EventBase registered by default?): %v", err)
	}
}
