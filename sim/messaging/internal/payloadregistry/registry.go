// Package payloadregistry shares the payload codec between messaging and its port implementations.
package payloadregistry

import "github.com/sarchlab/akita/v5/internal/codec"

// Registry stores payload types for checkpoint restoration. Contains uses an
// atomic snapshot load and map lookup, so Send does not take the registry lock.
var Registry = codec.NewRegistry[any]("payload")
