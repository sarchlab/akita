package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the ideal memory controller.
type Spec struct {
	Freq          timing.Freq `json:"freq"`
	Width         int         `json:"width"`
	Latency       int         `json:"latency"`
	CacheLineSize int         `json:"cache_line_size"`
	StorageRef    string      `json:"storage_ref"`
	AddrConvKind  string      `json:"addr_conv_kind"`

	AddrInterleavingSize    uint64 `json:"addr_interleaving_size"`
	AddrTotalNumOfElements  int    `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex int    `json:"addr_current_element_index"`
	AddrOffset              uint64 `json:"addr_offset"`
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

// State contains mutable runtime data for the ideal memory controller.
type State struct {
	InflightTransactions []inflightTransaction `json:"inflight_transactions"`
	CurrentState         string                `json:"current_state"`
	CurrentCmdID         uint64                `json:"current_cmd_id"`
	CurrentCmdSrc        messaging.RemotePort  `json:"current_cmd_src"`
}

// Resources holds the shared resources referenced by the memory controller.
type Resources struct {
	Storage *mem.Storage
}

// Comp is an ideal memory controller that always responds to a request in a
// fixed number of cycles, with no limit on concurrency. It is a
// modeling.Component specialized to this package's Spec, State, and Resources.
type Comp = modeling.Component[Spec, State, Resources]
