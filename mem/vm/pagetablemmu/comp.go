// Package pagetablemmu models a finite set of walkers over physical-memory
// radix page tables. Wire instances as local MMUs or authoritative fallback MMUs.
package pagetablemmu

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// BankSpec configures one non-leaf split translation cache bank. Zero sets
// and ways disable that bank.
type BankSpec struct {
	NumSets int `json:"num_sets"`
	NumWays int `json:"num_ways"`
}

// Spec describes one instance. Banks run from the root to the last non-leaf.
// The number of levels is len(Banks)+1, even when a bank is disabled.
type Spec struct {
	// Clock frequency used for lookup, filter and memory-issue cycles.
	Freq timing.Freq `json:"freq" akita:"min=1,max=1000000000000"`
	// Log2 of the common data page size in bytes.
	Log2PageSize uint `json:"log2_page_size" akita:"max=63"`
	// Width of the unsigned virtual address.
	VABits uint `json:"va_bits" akita:"min=1,max=64"`
	// One split-cache geometry per non-leaf radix level, root first.
	Banks []BankSpec `json:"banks"`
	// Maximum active PID/VPN pairs, including entries draining replies.
	NumWalkEntries int `json:"num_walk_entries" akita:"min=1"`
	// Maximum outstanding original messages per walk.
	MaxWaitersPerEntry int `json:"max_waiters_per_entry" akita:"min=1"`
	// Maximum new translations and outgoing translations/faults per cycle, each.
	AdmissionWidth int `json:"admission_width" akita:"min=1"`
	// Maximum PTE requests issued and replies consumed per cycle, each.
	MemoryWidth int `json:"memory_width" akita:"min=1"`
	// Parallel prefix probes admitted per bank per cycle.
	LookupWidth int `json:"lookup_width" akita:"min=1"`
	// Pointer fills admitted per bank per cycle.
	FillWidth int `json:"fill_width" akita:"min=1"`
	// Cycles between a parallel prefix probe and the first dependent read.
	LookupLatency int `json:"lookup_latency" akita:"min=1"`
	// Power-of-two SRAM bucket count. Zero disables the routing hint.
	FilterBuckets int `json:"filter_buckets" akita:"min=0"`
	// SRAM operations per cycle shared between lookups and maintenance.
	FilterWidth int `json:"filter_width"`
	// Cycles per SRAM lookup, insertion or displacement step.
	FilterLatency int `json:"filter_latency"`
	// Maximum displacements per insertion.
	FilterMaxKicks int `json:"filter_max_kicks"`
	// First 4 KiB-aligned physical address reserved for this instance.
	PoolBase uint64 `json:"pool_base"`
	// Number of contiguous 4 KiB nodes in the reserved pool.
	PoolNodes uint64 `json:"pool_nodes" akita:"min=1"`
	// Physical-memory destinations serving the node pool.
	Memory mem.AddressMapper `json:"memory"`
	// Responder that handles unresolved translations or placement decisions.
	FaultHandler messaging.RemotePort `json:"fault_handler"`
	// Consult the fault handler before using a mapping owned by another device.
	ConsultRemote bool `json:"consult_remote"`
}

// Ports separates ordinary translations, incoming and outgoing faults, PTE
// memory traffic and serialized maintenance. Only role-appropriate ports need
// traffic; bind all slots during setup.
type Ports struct {
	Top       messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol.responder"`
	FaultIn   messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol.responder"`
	PageFault messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol.requester"`
	Memory    messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	Control   messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol.responder"`
}

type waiter struct {
	ID      uint64
	Src     messaging.RemotePort
	VA      uint64
	Device  uint64
	FaultIn bool
	Task    uint64
}

// A decision lives only while this device has waiters for the active walk.
type decision struct {
	Device   uint64
	ID       uint64
	Task     uint64
	Response pagefaultprotocol.Rsp
}

type walk struct {
	Stage     walkStage
	FilterHit bool

	PID        vm.PID
	VPN        uint64
	Waiters    []waiter
	Decisions  []decision
	Level      int
	Table      uint64
	Due        timing.VTimeInPicoSec
	MemoryID   uint64
	MemoryTask uint64
	Page       vm.Page
}

type walkStage uint8

const (
	free walkStage = iota
	filterReady
	filterPending
	lookupReady
	lookupPending
	readReady
	readPending
	fillReady
	walkDone
)

type update struct {
	Request      pagetableprotocol.Req
	ID           uint64
	Src          messaging.RemotePort
	Task         uint64
	Stage        updateStage
	Error        string
	Level        int
	Table        uint64
	MemoryID     uint64
	MemoryTask   uint64
	Nodes        []uint64 // Pre-reserved chain below the first missing pointer.
	Child        int      // Index within Nodes being initialized.
	ZeroOffset   uint64
	FilterCursor uint64
	Kicks        int
	Displaced    fingerprint
	Alternate    uint64
	MoreFilter   bool
	Due          timing.VTimeInPicoSec
}

type state struct {
	Roots            map[vm.PID]uint64
	Owners           map[uint64]vm.PID
	NextNode         uint64
	Walks            []walk
	Banks            []prefixBank
	Filter           []fingerprint
	Update           update
	NextPipelineWalk int
	NextMemoryWalk   int
	PreferUpdate     bool
	PreferFaultIn    bool
	NextResponse     int
}

type middlewares struct {
	Control *controlMiddleware
	Walk    *walkMiddleware
	Memory  *memoryMiddleware
}

// Comp is the memory-backed MMU component.
type Comp = ticking.Component[Spec, state, modeling.None, Ports, middlewares]
