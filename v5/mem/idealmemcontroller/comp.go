package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the ideal memory controller.
type Spec struct {
	Width         int    `json:"width"`
	Latency       int    `json:"latency"`
	CacheLineSize int    `json:"cache_line_size"`
	StorageRef    string `json:"storage_ref"`
	AddrConvKind  string `json:"addr_conv_kind"`
}

// inflightTransaction tracks an in-progress memory request with a countdown.
type inflightTransaction struct {
	CycleLeft      int              `json:"cycle_left"`
	Address        uint64           `json:"address"`
	AccessByteSize uint64           `json:"access_byte_size"`
	ReqID          string           `json:"req_id"`
	IsRead         bool             `json:"is_read"`
	Data           []byte           `json:"data,omitempty"`
	DirtyMask      []bool           `json:"dirty_mask,omitempty"`
	Src            sim.RemotePort   `json:"src"`
}

// State contains mutable runtime data for the ideal memory controller.
type State struct {
	InflightTransactions []inflightTransaction `json:"inflight_transactions"`
	CurrentState         string                `json:"current_state"`
	CurrentCmdID         string                `json:"current_cmd_id"`
	CurrentCmdSrc        sim.RemotePort        `json:"current_cmd_src"`
}

// Comp is an ideal memory controller that can perform read and write.
// Ideal memory controller always responds to the request in a fixed number of
// cycles. There is no limitation on the concurrency of this unit.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort          sim.Port
	ctrlPort         sim.Port
	Storage          *mem.Storage
	addressConverter mem.AddressConverter
}

// GetStorage returns the underlying storage.
func (c *Comp) GetStorage() *mem.Storage {
	return c.Storage
}

// StorageName returns the name used to identify this component's storage.
func (c *Comp) StorageName() string {
	return c.GetSpec().StorageRef
}
