package messaging

import "github.com/sarchlab/akita/v5/codec"

// msgCodec decodes the polymorphic messages held in port buffers across a
// checkpoint. Each concrete message type is registered with RegisterMsg; the
// wire format and reflection machinery live in package codec.
var msgCodec = codec.NewRegistry[Msg]("message")

// RegisterMsg registers a concrete message type so a checkpoint that captured it
// in a port buffer can be decoded. Call it from an init() with a zero value of
// each message type, e.g. messaging.RegisterMsg(mem.ReadReq{}). Messages are
// value types, so register the value (a pointer also works). The tag is derived
// from the Go type, so checkpoints are restored by the same binary. Registering
// the same type twice is harmless.
//
// A forgotten registration fails loudly at load time, not silently: decoding a
// checkpoint that holds an unregistered message reports an unknown-message-type
// error.
func RegisterMsg(msg Msg) {
	msgCodec.Register(msg)
}

// CheckRoundTrip verifies that msg encodes and decodes back to an equal message
// of the same type. It is a test aid for a message-defining package to confirm
// its types are registered and serialize losslessly.
func CheckRoundTrip(msg Msg) error {
	return msgCodec.CheckRoundTrip(msg)
}
