package codec

import (
	"encoding/json"
	"strings"
	"testing"
)

// shape is a small interface to exercise the registry with both value and
// pointer concrete types.
type shape interface{ area() int }

type square struct {
	Side int `json:"side"`
}

func (s square) area() int { return s.Side * s.Side }

type rect struct {
	W int `json:"w"`
	H int `json:"h"`
}

func (r *rect) area() int { return r.W * r.H }

func TestRegistry_ValueAndPointerRoundTrip(t *testing.T) {
	r := NewRegistry[shape]("shape")
	r.Register(square{})
	r.Register(&rect{})

	in := []shape{square{Side: 3}, &rect{W: 2, H: 5}}

	encoded, err := EncodeSlice(in)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}

	out, err := r.DecodeSlice(encoded)
	if err != nil {
		t.Fatalf("DecodeSlice: %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("decoded %d shapes, want 2", len(out))
	}
	if _, ok := out[0].(square); !ok {
		t.Fatalf("out[0] = %T, want square (value form preserved)", out[0])
	}
	if _, ok := out[1].(*rect); !ok {
		t.Fatalf("out[1] = %T, want *rect (pointer form preserved)", out[1])
	}
	if out[0].area() != 9 || out[1].area() != 10 {
		t.Fatalf("areas = %d, %d; want 9, 10", out[0].area(), out[1].area())
	}
}

func TestRegistry_EmptySliceRoundTrip(t *testing.T) {
	r := NewRegistry[shape]("shape")

	encoded, err := EncodeSlice[shape](nil)
	if err != nil {
		t.Fatalf("EncodeSlice(nil): %v", err)
	}
	// On-disk form of an empty collection is a JSON empty array.
	if strings.TrimSpace(string(encoded)) != "[]" {
		t.Fatalf("empty encode = %q, want []", encoded)
	}

	out, err := r.DecodeSlice(encoded)
	if err != nil {
		t.Fatalf("DecodeSlice: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("decoded %d, want 0", len(out))
	}
}

func TestRegistry_SingleValueRoundTrip(t *testing.T) {
	r := NewRegistry[shape]("shape")
	r.Register(square{})
	r.Register(&rect{})

	encodedVal, err := Encode(square{Side: 3})
	if err != nil {
		t.Fatalf("Encode(value): %v", err)
	}
	gotVal, err := r.Decode(encodedVal)
	if err != nil {
		t.Fatalf("Decode(value): %v", err)
	}
	if _, ok := gotVal.(square); !ok {
		t.Fatalf("decoded %T, want square (value form preserved)", gotVal)
	}
	if gotVal.area() != 9 {
		t.Fatalf("area = %d, want 9", gotVal.area())
	}

	encodedPtr, err := Encode(&rect{W: 2, H: 5})
	if err != nil {
		t.Fatalf("Encode(pointer): %v", err)
	}
	gotPtr, err := r.Decode(encodedPtr)
	if err != nil {
		t.Fatalf("Decode(pointer): %v", err)
	}
	if _, ok := gotPtr.(*rect); !ok {
		t.Fatalf("decoded %T, want *rect (pointer form preserved)", gotPtr)
	}
}

// TestRegistry_EncodeIsSliceElement guards the invariant that Encode and
// EncodeSlice share one per-element wire format: a single Encode must equal the
// lone element of the corresponding one-element EncodeSlice.
func TestRegistry_EncodeIsSliceElement(t *testing.T) {
	// Encode needs no registry — encoding is registration-free — so this calls
	// the free functions directly, without constructing a Registry.
	single, err := Encode[shape](square{Side: 4})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	slice, err := EncodeSlice([]shape{square{Side: 4}})
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}

	// The slice form is a JSON array whose single element is the single form.
	want := "[" + string(single) + "]"
	if string(slice) != want {
		t.Fatalf("EncodeSlice = %s, want %s (single must be the slice element)",
			slice, want)
	}
}

func TestRegistry_DecodeUnknownSingleType(t *testing.T) {
	r := NewRegistry[shape]("shape")

	_, err := r.Decode(json.RawMessage(`{"type":"codec.square","payload":{}}`))
	if err == nil || !strings.Contains(err.Error(), "unknown shape type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}

func TestRegistry_UnknownType(t *testing.T) {
	r := NewRegistry[shape]("shape")

	_, err := r.DecodeSlice(json.RawMessage(`[{"type":"codec.square","payload":{}}]`))
	if err == nil || !strings.Contains(err.Error(), "unknown shape type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}

func TestRegistry_CheckRoundTrip(t *testing.T) {
	r := NewRegistry[shape]("shape")
	r.Register(square{})

	if err := r.CheckRoundTrip(square{Side: 4}); err != nil {
		t.Fatalf("CheckRoundTrip: %v", err)
	}

	// An unregistered type fails the round trip at decode.
	r2 := NewRegistry[shape]("shape")
	if err := r2.CheckRoundTrip(square{Side: 4}); err == nil {
		t.Fatalf("expected CheckRoundTrip to fail for an unregistered type")
	}
}
