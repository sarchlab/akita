package messaging

import "github.com/sarchlab/akita/v5/internal/codec"

// msgCodec registers payload types for checkpoint restoration. Contains uses
// an atomic snapshot load and map lookup, so Send never takes the registry lock.
var msgCodec = codec.NewRegistry[any]("payload")
