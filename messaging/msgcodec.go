package messaging

import "github.com/sarchlab/akita/v5/internal/codec"

// msgCodec decodes the polymorphic messages held in port buffers across a
// checkpoint. DefineProtocol registers each concrete message type a protocol
// carries; the wire format and reflection machinery live in package codec.
var msgCodec = codec.NewRegistry[Msg]("message")
