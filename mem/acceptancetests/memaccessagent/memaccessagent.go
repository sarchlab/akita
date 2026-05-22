// Package memaccessagent provides utility data structure definitions for
// writing memory system acceptance tests.
package memaccessagent

import (
	"encoding/binary"
	"math/rand"

	"github.com/sarchlab/akita/v5/daisen"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

var dumpLog = false

// A MemAccessAgent is a Component that can help testing the cache and the
// memory controllers by generating a large number of read and write requests.
type MemAccessAgent struct {
	*modeling.Component[Spec, State]

	// LowModule is the downstream port to which memory requests are sent.
	// It is not serialized as part of the state.
	LowModule messaging.Port

	// Rand is the random source used by the agent. If nil, the global
	// math/rand functions are used (non-deterministic in Go 1.22+).
	Rand *rand.Rand

	WriteProgressBar *daisen.ProgressBar
	ReadProgressBar  *daisen.ProgressBar
}

func bytesToUint32(data []byte) uint32 {
	a := uint32(0)
	a += uint32(data[0])
	a += uint32(data[1]) << 8
	a += uint32(data[2]) << 16
	a += uint32(data[3]) << 24

	return a
}

func uint32ToBytes(data uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, data)

	return bytes
}

// globalFloat64 returns a random float64 from the global rand source.
func globalFloat64() float64 {
	return rand.Float64()
}

// globalUint64 returns a random uint64 from the global rand source.
func globalUint64() uint64 {
	return rand.Uint64()
}

// globalUint32 returns a random uint32 from the global rand source.
func globalUint32() uint32 {
	return rand.Uint32()
}
