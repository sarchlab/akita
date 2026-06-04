package messaging

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// TypedPayload is the serialized form of a polymorphic value: a type tag plus
// the JSON encoding of the concrete value. It lets a heterogeneous container
// (such as a port buffer of Msg) be checkpointed and decoded back to the right
// concrete types.
type TypedPayload struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var (
	msgRegistryMu sync.RWMutex
	msgRegistry   = map[string]reflect.Type{}
)

// RegisterMsg registers a concrete message type so it can be encoded and decoded
// for checkpoints. Call it from an init() with a zero value of each message
// type, e.g. messaging.RegisterMsg(mem.ReadReq{}). Messages are value types, so
// register the value (a pointer also works). The tag is derived from the Go
// type, so checkpoints are restored by the same binary. Registering the same
// type twice is harmless.
func RegisterMsg(msg Msg) {
	t := reflect.TypeOf(msg)
	if t == nil {
		panic("messaging: RegisterMsg requires a non-nil message")
	}

	msgRegistryMu.Lock()
	msgRegistry[t.String()] = t
	msgRegistryMu.Unlock()
}

// EncodeMsg encodes a message into a TypedPayload.
func EncodeMsg(msg Msg) (TypedPayload, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return TypedPayload{}, fmt.Errorf("messaging: encode %T: %w", msg, err)
	}

	return TypedPayload{
		Type:    reflect.TypeOf(msg).String(),
		Payload: payload,
	}, nil
}

// DecodeMsg decodes a TypedPayload back into a message of its registered type,
// in the same value or pointer form the type was registered as.
func DecodeMsg(tp TypedPayload) (Msg, error) {
	msgRegistryMu.RLock()
	t, ok := msgRegistry[tp.Type]
	msgRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(
			"messaging: unknown message type %q (call messaging.RegisterMsg for it)",
			tp.Type)
	}

	elem := t
	if t.Kind() == reflect.Ptr {
		elem = t.Elem()
	}
	ptr := reflect.New(elem)

	if err := json.Unmarshal(tp.Payload, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("messaging: decode %s: %w", tp.Type, err)
	}

	result := ptr.Interface()
	if t.Kind() != reflect.Ptr {
		result = ptr.Elem().Interface()
	}

	msg, ok := result.(Msg)
	if !ok {
		return nil, fmt.Errorf("messaging: %s does not implement Msg", tp.Type)
	}

	return msg, nil
}
