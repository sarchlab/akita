// Package codec provides a generic, type-safe registry for serializing
// heterogeneous collections of polymorphic values to JSON and decoding them back
// into their concrete types.
//
// It is the shared machinery behind the message codec in package messaging and
// the event codec in package timing. Each owning package instantiates its own
// Registry for its interface type (Registry[messaging.Msg],
// Registry[timing.Event]) and re-exports a thin Register wrapper; the wire
// format never leaves this package.
//
// The registry exists because Go has no runtime "construct a value of the type
// named X": to decode a heterogeneous container of an interface type (a port
// buffer of Msg, the engine's queue of Event) each element must be tagged with
// its concrete type name and that name resolved back to a reflect.Type that was
// registered earlier. Encoding needs no registration; decoding does.
package codec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// typedPayload is the serialized form of a single polymorphic value: a type tag
// plus the JSON encoding of the concrete value. It is unexported and never
// leaves this package — callers work with whole slices through EncodeSlice and
// DecodeSlice, so the wire format stays hidden.
type typedPayload struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Registry maps concrete type names to their reflect.Type so that values of an
// interface type T (e.g. messaging.Msg or timing.Event) can be reconstructed
// from a checkpoint. A Registry is safe for concurrent use.
type Registry[T any] struct {
	// label is the domain noun used in error messages, e.g. "message" or
	// "event", so a failure reads "unknown message type ...".
	label string

	mu    sync.RWMutex
	types map[string]reflect.Type
}

// NewRegistry returns an empty Registry. The label is a short domain noun (e.g.
// "message", "event") that appears in error messages.
func NewRegistry[T any](label string) *Registry[T] {
	return &Registry[T]{
		label: label,
		types: map[string]reflect.Type{},
	}
}

// Tag returns the wire tag the registry uses for the concrete type of v. It is
// the full import path plus the type name (e.g.
// "github.com/sarchlab/akita/v5/mem/memprotocol.ReadReq"), with a "*" prefix
// for pointer forms, so same-named types in different packages cannot collide.
// Exposed so audits can compute the tag a value would be stored under.
func Tag(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		panic("codec: Tag requires a non-nil value")
	}

	return tagOf(t)
}

// tagOf derives the wire tag from a reflect.Type. It must be used identically
// on the register and encode paths.
func tagOf(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		return "*" + tagOf(t.Elem())
	}

	if t.PkgPath() == "" {
		// Unnamed or predeclared types have no import path; fall back to the
		// short name (e.g. for tests using local anonymous helpers).
		return t.String()
	}

	return t.PkgPath() + "." + t.Name()
}

// Register records a concrete type so it can be decoded later. Pass a zero value
// of the concrete type, in the value or pointer form it will be encoded as; the
// tag is derived from the Go type. Registering the same type twice is harmless.
func (r *Registry[T]) Register(v T) {
	t := reflect.TypeOf(v)
	if t == nil {
		panic(fmt.Sprintf("codec: Register requires a non-nil %s", r.label))
	}

	r.mu.Lock()
	r.types[tagOf(t)] = t
	r.mu.Unlock()
}

// Tags returns the wire tags of all registered types, in no particular order.
// It exists for coverage audits; it is not used on the checkpoint path.
func (r *Registry[T]) Tags() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tags := make([]string, 0, len(r.types))
	for tag := range r.types {
		tags = append(tags, tag)
	}

	return tags
}

// Encode encodes a single value of T into one typed payload: a type tag plus
// the JSON encoding of the concrete value. Encoding is registration-free and
// stateless — only decoding consults a Registry's type map — so Encode is a
// plain generic function, not a Registry method. EncodeSlice is the same
// operation over many values.
//
// A nil value (an absent optional payload, e.g. a flit that carries no message)
// encodes as JSON null, and Decode maps null back to the zero T. This policy
// lives here rather than in the messaging/timing wrappers because an optional
// polymorphic value is common to every interface the codec serves. Only this
// single-value path is nil-tolerant; see EncodeSlice.
func Encode[T any](v T) (json.RawMessage, error) {
	if any(v) == nil {
		return json.RawMessage("null"), nil
	}

	tp, err := encodeOne(v)
	if err != nil {
		return nil, err
	}

	return json.Marshal(tp)
}

// EncodeSlice encodes a slice of T into a JSON array of typed payloads. Like
// Encode it needs no Registry: each element is tagged with its concrete type and
// marshalled with the default JSON encoding. Unlike Encode it does not
// special-case a nil element; the collections it serializes (port buffers, the
// event queue) never contain one.
func EncodeSlice[T any](vs []T) (json.RawMessage, error) {
	payloads := make([]typedPayload, len(vs))
	for i, v := range vs {
		tp, err := encodeOne(v)
		if err != nil {
			return nil, err
		}
		payloads[i] = tp
	}

	return json.Marshal(payloads)
}

// encodeOne builds the typed payload for a single value: the concrete type's tag
// plus its JSON. Encode and EncodeSlice both build on it, so the per-element wire
// format is defined in exactly one place.
func encodeOne[T any](v T) (typedPayload, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return typedPayload{}, fmt.Errorf("codec: encode %T: %w", v, err)
	}

	return typedPayload{
		Type:    tagOf(reflect.TypeOf(v)),
		Payload: raw,
	}, nil
}

// Decode decodes a single typed payload produced by Encode back into a concrete
// value of its registered type, in the same value or pointer form its type was
// registered as. Unlike Encode it is a Registry method: decoding resolves the
// type tag through the registry's map. It is the single-value counterpart to
// DecodeSlice; both build on decodeOne.
//
// Reversing Encode's nil policy, a null or empty payload is an absent optional
// value and decodes to the zero T (nil for an interface type).
func (r *Registry[T]) Decode(data json.RawMessage) (T, error) {
	var zero T

	if len(data) == 0 || string(data) == "null" {
		return zero, nil
	}

	var tp typedPayload
	if err := json.Unmarshal(data, &tp); err != nil {
		return zero, fmt.Errorf("codec: decode %s: %w", r.label, err)
	}

	return r.decodeOne(tp)
}

// DecodeSlice decodes a JSON array produced by EncodeSlice back into concrete
// values of their registered types, each in the same value or pointer form its
// type was registered as.
func (r *Registry[T]) DecodeSlice(data json.RawMessage) ([]T, error) {
	var payloads []typedPayload
	if err := json.Unmarshal(data, &payloads); err != nil {
		return nil, fmt.Errorf("codec: decode %s list: %w", r.label, err)
	}

	out := make([]T, len(payloads))
	for i, tp := range payloads {
		v, err := r.decodeOne(tp)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}

	return out, nil
}

func (r *Registry[T]) decodeOne(tp typedPayload) (T, error) {
	var zero T

	r.mu.RLock()
	t, ok := r.types[tp.Type]
	r.mu.RUnlock()
	if !ok {
		return zero, fmt.Errorf(
			"codec: unknown %s type %q (register it before checkpointing)",
			r.label, tp.Type)
	}

	// Allocate a *Concrete to unmarshal into, regardless of whether the type was
	// registered as a value or a pointer.
	elem := t
	if t.Kind() == reflect.Ptr {
		elem = t.Elem()
	}
	ptr := reflect.New(elem)

	if err := json.Unmarshal(tp.Payload, ptr.Interface()); err != nil {
		return zero, fmt.Errorf("codec: decode %s %s: %w", r.label, tp.Type, err)
	}

	// Return the same value/pointer form the type was registered as.
	result := ptr.Interface()
	if t.Kind() != reflect.Ptr {
		result = ptr.Elem().Interface()
	}

	v, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf(
			"codec: %s %s does not implement the registry type", r.label, tp.Type)
	}

	return v, nil
}

// CheckRoundTrip verifies that v encodes and decodes back to an equal value of
// the same concrete type. It is a test aid for confirming a type is registered
// and serializes losslessly; it is not used on the checkpoint hot path.
func (r *Registry[T]) CheckRoundTrip(v T) error {
	encoded, err := EncodeSlice([]T{v})
	if err != nil {
		return err
	}

	decoded, err := r.DecodeSlice(encoded)
	if err != nil {
		return err
	}

	if len(decoded) != 1 {
		return fmt.Errorf(
			"codec: %s round trip produced %d values, want 1", r.label, len(decoded))
	}

	if !reflect.DeepEqual(v, decoded[0]) {
		return fmt.Errorf(
			"codec: %s round trip mismatch: got %+v, want %+v",
			r.label, decoded[0], v)
	}

	return nil
}
