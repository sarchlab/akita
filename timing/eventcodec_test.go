package timing

import (
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

	tp, err := EncodeEvent(e)
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if tp.Type != "timing.valueEvent" {
		t.Fatalf("type tag = %q", tp.Type)
	}

	got, err := DecodeEvent(tp)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	gv, ok := got.(valueEvent)
	if !ok {
		t.Fatalf("decoded type = %T, want value valueEvent", got)
	}
	if gv.Payload != 42 || gv.Time() != 10 || gv.HandlerID() != "h" {
		t.Fatalf("decoded event mismatch: %+v", gv)
	}
}

func TestEventRegistryPointerRoundTrip(t *testing.T) {
	RegisterEvent(&pointerEvent{})

	e := &pointerEvent{Tag: "x"}
	e.Time_ = 20

	tp, err := EncodeEvent(e)
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if tp.Type != "*timing.pointerEvent" {
		t.Fatalf("type tag = %q", tp.Type)
	}

	got, err := DecodeEvent(tp)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	gp, ok := got.(*pointerEvent)
	if !ok {
		t.Fatalf("decoded type = %T, want *pointerEvent", got)
	}
	if gp.Tag != "x" || gp.Time() != 20 {
		t.Fatalf("decoded event mismatch: %+v", gp)
	}
}

func TestEventRegistryUnknownType(t *testing.T) {
	_, err := DecodeEvent(TypedPayload{Type: "nope.Type", Payload: []byte("{}")})
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}
