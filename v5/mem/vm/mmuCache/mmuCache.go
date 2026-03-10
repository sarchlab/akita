package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/vm/mmuCache/internal"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the mmuCache.
type Spec struct {
	NumBlocks       int    `json:"num_blocks"`
	NumLevels       int    `json:"num_levels"`
	PageSize        uint64 `json:"page_size"`
	Log2PageSize    uint64 `json:"log2_page_size"`
	NumReqPerCycle  int    `json:"num_req_per_cycle"`
	LatencyPerLevel uint64 `json:"latency_per_level"`
}

// State contains mutable runtime data for the mmuCache.
// Runtime data with pointers/interfaces stays on the Comp struct.
type State struct{}

type Comp struct {
	*modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	UpModule  sim.Port
	LowModule sim.Port
	state     string

	table []internal.Set

	inflightFlushReq *sim.Msg // payload: *FlushReqPayload
}

func (c *Comp) reset() {
	spec := c.GetSpec()
	c.table = make([]internal.Set, spec.NumLevels)
	for i := 0; i < spec.NumLevels; i++ {
		c.table[i] = internal.NewSet(spec.NumBlocks)
	}
}
