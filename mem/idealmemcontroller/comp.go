package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the ideal memory controller.
type Spec struct {
	Freq          timing.Freq `json:"freq"`
	Width         int         `json:"width"`
	Latency       int         `json:"latency"`
	CacheLineSize int         `json:"cache_line_size"`
}

// inflightTransaction tracks an in-progress memory request with a countdown.
type inflightTransaction struct {
	CycleLeft      int                  `json:"cycle_left"`
	Address        uint64               `json:"address"`
	AccessByteSize uint64               `json:"access_byte_size"`
	ReqID          uint64               `json:"req_id"`
	RecvTaskID     uint64               `json:"recv_task_id"`
	IsRead         bool                 `json:"is_read"`
	Data           []byte               `json:"data,omitempty"`
	DirtyMask      []bool               `json:"dirty_mask,omitempty"`
	Src            messaging.RemotePort `json:"src"`
}

// state contains mutable runtime data for the ideal memory controller.
type state struct {
	InflightTransactions []inflightTransaction    `json:"inflight_transactions"`
	ControlState         memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID         uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc        messaging.RemotePort     `json:"current_cmd_src"`
}

// Resources holds the shared resources referenced by the memory controller,
// supplied by the system builder.
type Resources struct {
	// Storage holds the memory's data. It is required; the system builder
	// sizes it and may share it with other components.
	Storage *mem.Storage
}

// Ports holds the memory controller's ports.
type Ports struct {
	// Top receives read and write requests and returns their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Control receives enable, pause, drain, and reset commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the memory controller's behavior, run in field order
// every cycle. Control runs first so that a Pause, Drain, or Reset takes
// effect before any Top traffic is admitted in the same cycle.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// Memory admits requests, counts down their latency, and responds.
	Memory *memMiddleware
}

// Comp is an ideal memory controller that always responds to a request in a
// fixed number of cycles, with no limit on concurrency. It is a ticking
// component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
