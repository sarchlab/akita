// Package memaccessagent provides utility data structure definitions for
// writing memory system acceptance tests.
package memaccessagent

import (
	"encoding/binary"
	"math/rand"

	"github.com/sarchlab/akita/v5/daisen2"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var dumpLog = false

// Spec contains the immutable configuration for the MemAccessAgent.
type Spec struct {
	Freq              timing.Freq `json:"freq"`
	MaxAddress        uint64      `json:"max_address"`
	UseVirtualAddress bool        `json:"use_virtual_address"`
}

// State contains the mutable runtime data for the MemAccessAgent.
type State struct {
	WriteLeft       int                     `json:"write_left"`
	ReadLeft        int                     `json:"read_left"`
	KnownMemValue   map[uint64][]uint32     `json:"known_mem_value"`
	PendingReadReq  map[uint64]mem.ReadReq  `json:"pending_read_req"`
	PendingWriteReq map[uint64]mem.WriteReq `json:"pending_write_req"`
}

// A MemAccessAgent is a Component that can help testing the cache and the
// memory controllers by generating a large number of read and write requests.
type MemAccessAgent struct {
	*modeling.Component[Spec, State, modeling.None]

	// LowModule is the downstream port to which memory requests are sent.
	// It is not serialized as part of the state.
	LowModule messaging.Port

	// Rand is the random source used by the agent. If nil, the global
	// math/rand functions are used (non-deterministic in Go 1.22+).
	Rand *rand.Rand

	WriteProgressBar *daisen2.ProgressBar
	ReadProgressBar  *daisen2.ProgressBar
}

// CreateProgressBars creates the read/write progress bars for the agent.
func (a *MemAccessAgent) CreateProgressBars(
	createProgressBar func(name string, total uint64) *daisen2.ProgressBar,
) {
	if createProgressBar == nil {
		return
	}

	writeTotal := remainingAccesses(
		a.State.WriteLeft,
		len(a.State.PendingWriteReq),
	)
	readTotal := remainingAccesses(
		a.State.ReadLeft,
		len(a.State.PendingReadReq),
	)

	if writeTotal > 0 && a.WriteProgressBar == nil {
		a.WriteProgressBar = createProgressBar(a.Name()+".Writes", writeTotal)
	}

	if readTotal > 0 && a.ReadProgressBar == nil {
		a.ReadProgressBar = createProgressBar(a.Name()+".Reads", readTotal)
	}
}

func remainingAccesses(left, pending int) uint64 {
	total := left + pending
	if total <= 0 {
		return 0
	}

	return uint64(total)
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
