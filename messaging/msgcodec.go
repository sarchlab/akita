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
// for checkpoints. Call it from an init() with a zero pointer of each message
// type, e.g. messaging.RegisterMsg(&mem.ReadReq{}). The type tag is derived from
// the Go type, so checkpoints are restored by the same binary that produced
// them. Registering the same type twice is harmless.
func RegisterMsg(msg Msg) {
	t := reflect.TypeOf(msg)
	if t == nil || t.Kind() != reflect.Ptr {
		panic(fmt.Sprintf(
			"messaging: RegisterMsg requires a non-nil pointer message, got %T", msg))
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

// DecodeMsg decodes a TypedPayload back into a message of its registered type.
func DecodeMsg(tp TypedPayload) (Msg, error) {
	msgRegistryMu.RLock()
	t, ok := msgRegistry[tp.Type]
	msgRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(
			"messaging: unknown message type %q (call messaging.RegisterMsg for it)",
			tp.Type)
	}

	ptr := reflect.New(t.Elem()) // a fresh *ConcreteMsg
	if err := json.Unmarshal(tp.Payload, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("messaging: decode %s: %w", tp.Type, err)
	}

	msg, ok := ptr.Interface().(Msg)
	if !ok {
		return nil, fmt.Errorf("messaging: %s does not implement Msg", tp.Type)
	}

	return msg, nil
}
