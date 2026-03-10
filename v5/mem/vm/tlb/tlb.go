package tlb

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

const (
	tlbStateEnable = "enable"
	tlbStatePause  = "pause"
	tlbStateDrain  = "drain"
	tlbStateFlush  = "flush"
)

// Spec contains immutable configuration for the TLB.
type Spec struct {
	NumSets        int    `json:"num_sets"`
	NumWays        int    `json:"num_ways"`
	PageSize       uint64 `json:"page_size"`
	NumReqPerCycle int    `json:"num_req_per_cycle"`
}

// State contains mutable runtime data for the TLB.
type State struct {
	TLBState           string                `json:"tlb_state"`
	Sets               []setState            `json:"sets"`
	MSHREntries        []mshrEntryState      `json:"mshr_entries"`
	HasRespondingMSHR  bool                  `json:"has_responding_mshr"`
	RespondingMSHRData mshrEntryState        `json:"responding_mshr_data"`
	PipelineStages     []pipelineStageState  `json:"pipeline_stages"`
	BufferItems        []pipelineTLBReqState `json:"buffer_items"`
	HasInflightFlushReq bool                 `json:"has_inflight_flush_req"`
	InflightFlushReqMsg msgRef               `json:"inflight_flush_req_msg"`
}

// Comp is a Translation Lookaside Buffer (TLB) that stores part of the page
// table.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	addressMapper mem.AddressToPortMapper

	state string

	sets []internal.Set

	mshr                mshr
	respondingMSHREntry *mshrEntry
	responsePipeline    queueing.Pipeline
	responseBuffer      queueing.Buffer

	inflightFlushReq *sim.Msg // payload: *FlushReqPayload
}

// reset sets all the entries in the TLB to be invalid
func (c *Comp) reset() {
	spec := c.GetSpec()
	c.sets = make([]internal.Set, spec.NumSets)
	for i := 0; i < spec.NumSets; i++ {
		set := internal.NewSet(spec.NumWays)
		c.sets[i] = set
	}
}
