// Package memaccessagent provides utility data structure definitions for
// writing memory system acceptance tests.
package memaccessagent

import (
	"encoding/binary"

	"github.com/sarchlab/akita/v5/daisen2"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

var dumpLog = false

// Spec contains the immutable configuration for the MemAccessAgent.
type Spec struct {
	Freq       timing.Freq `json:"freq"`
	MaxAddress uint64      `json:"max_address"`
	WriteLeft  int         `json:"write_left"`
	ReadLeft   int         `json:"read_left"`

	// AddressOffset is added to every generated address. It lets several
	// agents that share a downstream memory exercise disjoint address ranges:
	// configure agent i with AddressOffset = i*MaxAddress so that each agent
	// only ever touches [AddressOffset, AddressOffset+MaxAddress). Defaults to
	// 0, which keeps single-agent tests unchanged.
	AddressOffset uint64 `json:"address_offset"`

	// RandSeed seeds the agent's private random source, so a run is
	// reproducible from the seed: the agent draws the same values as
	// math/rand.New(math/rand.NewSource(RandSeed)). Every value, including 0,
	// is a seed. The default is a fixed seed, so the default access stream is
	// deterministic.
	RandSeed int64 `json:"rand_seed"`
}

// Resources holds the external wiring referenced by the MemAccessAgent.
type Resources struct {
	// LowModule is the downstream port to which memory requests are sent. It
	// is required.
	LowModule messaging.Port
}

// State contains the mutable runtime data for the MemAccessAgent.
type State struct {
	WriteLeft       int                      `json:"write_left"`
	ReadLeft        int                      `json:"read_left"`
	KnownMemValue   map[uint64][]uint32      `json:"known_mem_value"`
	PendingReadReq  map[uint64]messaging.Msg `json:"pending_read_req"`
	PendingWriteReq map[uint64]messaging.Msg `json:"pending_write_req"`
	RNG             randState                `json:"rng"`
}

// Ports holds the MemAccessAgent's ports.
type Ports struct {
	// Mem sends read and write requests to the LowModule and receives their
	// responses.
	Mem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}

// Middlewares holds the MemAccessAgent's behavior, run every cycle.
type Middlewares struct {
	// Agent checks responses and issues new read and write requests.
	Agent *agentMiddleware
}

// Comp is the MemAccessAgent component: a ticking component that helps test
// caches and memory controllers by generating a large number of read and
// write requests and checking the data read back.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]

// A MemAccessAgent is a Component that can help testing the cache and the
// memory controllers by generating a large number of read and write requests.
type MemAccessAgent = Comp

// CreateProgressBars creates the read/write progress bars for the agent a.
// Progress bars observe the run; they are not simulation state and are not
// checkpointed. Call it after Build, like attaching a hook.
func CreateProgressBars(
	a *Comp,
	createProgressBar func(name string, total uint64) *daisen2.ProgressBar,
) {
	if createProgressBar == nil {
		return
	}

	mw := a.Middlewares.Agent

	writeTotal := remainingAccesses(
		a.State.WriteLeft,
		len(a.State.PendingWriteReq),
	)
	readTotal := remainingAccesses(
		a.State.ReadLeft,
		len(a.State.PendingReadReq),
	)

	if writeTotal > 0 && mw.writeProgressBar == nil {
		mw.writeProgressBar = createProgressBar(a.Name()+".Writes", writeTotal)
	}

	if readTotal > 0 && mw.readProgressBar == nil {
		mw.readProgressBar = createProgressBar(a.Name()+".Reads", readTotal)
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
