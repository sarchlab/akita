package timing

import "github.com/sarchlab/akita/v5/internal/codec"

// eventCodec decodes the polymorphic events held in the engine's queues across a
// checkpoint. Each concrete event type is registered with RegisterEvent; the
// wire format and reflection machinery live in package codec.
var eventCodec = codec.NewRegistry[Event]("event")

// init registers the built-in EventBase so a checkpoint can round-trip a plain
// event scheduled via MakeEventBase. Concrete events that embed EventBase
// register their own outer type (e.g. modeling.TickEvent) separately.
func init() {
	RegisterEvent(EventBase{})
}

// RegisterEvent registers a concrete event type so a checkpoint that captured it
// in the engine queue can be decoded. Call it from an init() with a zero value
// of each event type. Events may be value types (e.g. modeling.TickEvent) or
// pointers; the tag is derived from the Go type either way. Registering the same
// type twice is harmless.
//
// A forgotten registration fails loudly at load time: decoding a checkpoint that
// holds an unregistered event reports an unknown-event-type error.
func RegisterEvent(evt Event) {
	eventCodec.Register(evt)
}

// CheckRoundTrip verifies that evt encodes and decodes back to an equal event of
// the same type. It is a test aid for an event-defining package to confirm its
// types are registered and serialize losslessly.
func CheckRoundTrip(evt Event) error {
	return eventCodec.CheckRoundTrip(evt)
}
