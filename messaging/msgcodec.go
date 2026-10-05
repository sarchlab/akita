package messaging

import "github.com/sarchlab/akita/v5/internal/codec"

// msgCodec registers payloads during package initialization. Runtime reads,
// including the Send validation path, are plain map lookups.
var msgCodec = codec.NewStaticRegistry[any]("payload")
