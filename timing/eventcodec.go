package timing

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// TypedPayload is the serialized form of a polymorphic event: a type tag plus
// the JSON encoding of the concrete event. It lets the engine's event queue be
// checkpointed and decoded back to the right concrete event types.
type TypedPayload struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var (
	eventRegistryMu sync.RWMutex
	eventRegistry   = map[string]reflect.Type{}
)

// RegisterEvent registers a concrete event type so it can be encoded and decoded
// for checkpoints. Call it from an init() with a zero value of each event type.
// Events may be value types (e.g. modeling.TickEvent) or pointers; the tag is
// derived from the Go type either way. Registering the same type twice is
// harmless.
func RegisterEvent(evt Event) {
	t := reflect.TypeOf(evt)
	if t == nil {
		panic("timing: RegisterEvent requires a non-nil event")
	}

	eventRegistryMu.Lock()
	eventRegistry[t.String()] = t
	eventRegistryMu.Unlock()
}

// EncodeEvent encodes an event into a TypedPayload.
func EncodeEvent(evt Event) (TypedPayload, error) {
	payload, err := json.Marshal(evt)
	if err != nil {
		return TypedPayload{}, fmt.Errorf("timing: encode %T: %w", evt, err)
	}

	return TypedPayload{
		Type:    reflect.TypeOf(evt).String(),
		Payload: payload,
	}, nil
}

// DecodeEvent decodes a TypedPayload back into an event of its registered type.
func DecodeEvent(tp TypedPayload) (Event, error) {
	eventRegistryMu.RLock()
	t, ok := eventRegistry[tp.Type]
	eventRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(
			"timing: unknown event type %q (call timing.RegisterEvent for it)",
			tp.Type)
	}

	// Allocate a *Concrete to unmarshal into, regardless of whether the event
	// was registered as a value or pointer type.
	elem := t
	if t.Kind() == reflect.Ptr {
		elem = t.Elem()
	}
	ptr := reflect.New(elem)

	if err := json.Unmarshal(tp.Payload, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("timing: decode %s: %w", tp.Type, err)
	}

	// Return the same value/pointer form the type was registered as.
	result := ptr.Interface()
	if t.Kind() != reflect.Ptr {
		result = ptr.Elem().Interface()
	}

	evt, ok := result.(Event)
	if !ok {
		return nil, fmt.Errorf("timing: %s does not implement Event", tp.Type)
	}

	return evt, nil
}
