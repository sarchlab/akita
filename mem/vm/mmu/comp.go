package mmu

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the MMU.
type Spec struct {
	Freq                timing.Freq `json:"freq"`
	Latency             int         `json:"latency"`
	MaxRequestsInFlight int         `json:"max_requests_in_flight"`
	AutoPageAllocation  bool        `json:"auto_page_allocation"`
	Log2PageSize        uint64      `json:"log2_page_size"`
}

// Ports holds the MMU's ports.
type Ports struct {
	// Top receives translation requests and returns their responses.
	Top messaging.Port `akita:"role=vm/responder"`

	// Control receives enable, pause, drain, flush, and reset commands.
	Control messaging.Port `akita:"role=mem.control/responder"`
}

// Middlewares holds the MMU's behavior, run in field order every cycle.
type Middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// Translation accepts translation requests, walks the page table, and
	// responds.
	Translation *translationMW
}

// transactionState is the canonical transaction representation.
type transactionState struct {
	ReqID        uint64               `json:"req_id"`
	RecvTaskID   uint64               `json:"recv_task_id"`
	ReqSrc       messaging.RemotePort `json:"req_src"`
	ReqDst       messaging.RemotePort `json:"req_dst"`
	PID          uint32               `json:"pid"`
	VAddr        uint64               `json:"vaddr"`
	DeviceID     uint64               `json:"device_id"`
	TransLatency uint64               `json:"trans_latency"`
	Page         vm.Page              `json:"page"`
	CycleLeft    int                  `json:"cycle_left"`
	// WalkTaskID is the pipeline subtask that spans the local page-table-walk
	// latency (the CycleLeft countdown), a child of the req_in. It pairs with
	// the ".walk" work milestone so the walk renders as a child bar rather than
	// a bare work interval.
	WalkTaskID uint64 `json:"walk_task_id"`
}

// State contains mutable runtime data for the MMU.
type State struct {
	ControlState        memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID        uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort     `json:"current_cmd_src"`
	WalkingTranslations []transactionState       `json:"walking_translations"`
	NextPhysicalPage    uint64                   `json:"next_physical_page"`
	ToRemoveFromPTW     []int                    `json:"to_remove_from_ptw"`
}

// Resources holds the shared resources referenced by the MMU. The page table is
// external wiring shared with other components.
type Resources struct {
	// PageTable holds the translations the MMU walks. It is required, and its
	// page size must match Spec.Log2PageSize.
	PageTable vm.PageTable `json:"-"`
}

// Comp is the MMU component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
