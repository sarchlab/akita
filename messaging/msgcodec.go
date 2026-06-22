package messaging

import (
	"encoding/json"

	"github.com/sarchlab/akita/v5/internal/codec"
)

// msgCodec decodes the polymorphic messages held in port buffers across a
// checkpoint. Each concrete message type is registered with RegisterMsg; the
// wire format and reflection machinery live in package codec.
var msgCodec = codec.NewRegistry[Msg]("message")

// EncodeMsg encodes a single message into a self-describing JSON payload that
// preserves its concrete type, so DecodeMsg can reconstruct it. It is the public
// entry point for callers that carry one polymorphic message inside an otherwise
// plain-JSON structure (e.g. a flit payload, or an endpoint's reassembly state),
// so they need not depend on package codec directly. A nil message encodes as
// JSON null; that policy lives in codec.Encode, shared by every interface, so
// this is a thin delegate. The concrete type must be registered (see RegisterMsg
// / DefineProtocol) for DecodeMsg to restore it.
func EncodeMsg(msg Msg) (json.RawMessage, error) {
	return codec.Encode(msg)
}

// DecodeMsg reverses EncodeMsg, resolving the message's concrete type through the
// message registry — which is why, unlike EncodeMsg, it cannot be a free
// function. A null or empty payload decodes to a nil message. A payload whose
// concrete type was never registered fails loudly with an unknown-message-type
// error, matching the port-buffer checkpoint path.
func DecodeMsg(data json.RawMessage) (Msg, error) {
	return msgCodec.Decode(data)
}

// RegisterMsg registers a concrete message type so a checkpoint that captured it
// in a port buffer can be decoded. It is the low-level primitive behind
// DefineProtocol, which is the recommended way to register messages: a protocol
// definition registers every message type it carries in one declaration.
// Messages are value types, so register the value (a pointer also works). The
// tag is derived from the Go type, so checkpoints are restored by the same
// binary. Registering the same type twice is harmless.
//
// A forgotten registration fails loudly at load time, not silently: decoding a
// checkpoint that holds an unregistered message reports an unknown-message-type
// error.
func RegisterMsg(msg Msg) {
	msgCodec.Register(msg)
}
