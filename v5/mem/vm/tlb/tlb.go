package tlb

import (
	"github.com/sarchlab/akita/v5/modeling"
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
	NumSets                    int              `json:"num_sets"`
	NumWays                    int              `json:"num_ways"`
	PageSize                   uint64           `json:"page_size"`
	NumReqPerCycle             int              `json:"num_req_per_cycle"`
	MSHRSize                   int              `json:"mshr_size"`
	Latency                    int              `json:"latency"`
	PipelineWidth              int              `json:"pipeline_width"`
	AddrMapperKind             string           `json:"addr_mapper_kind"`
	AddrMapperPorts            []sim.RemotePort `json:"addr_mapper_ports"`
	AddrMapperInterleavingSize uint64           `json:"addr_mapper_interleaving_size"`
}

// State contains mutable runtime data for the TLB.
type State struct {
	TLBState            string                `json:"tlb_state"`
	Sets                []setState            `json:"sets"`
	MSHREntries         []mshrEntryState      `json:"mshr_entries"`
	HasRespondingMSHR   bool                  `json:"has_responding_mshr"`
	RespondingMSHRData  mshrEntryState        `json:"responding_mshr_data"`
	PipelineStages      []pipelineStageState  `json:"pipeline_stages"`
	BufferItems         []pipelineTLBReqState `json:"buffer_items"`
	HasInflightFlushReq bool                  `json:"has_inflight_flush_req"`
	InflightFlushReqMsg FlushReq              `json:"inflight_flush_req_msg"`
	PipelineNumStages   int                   `json:"pipeline_num_stages"`
}

// Comp is a Translation Lookaside Buffer (TLB) that stores part of the page
// table.
type Comp struct {
	*modeling.Component[Spec, State]
}
