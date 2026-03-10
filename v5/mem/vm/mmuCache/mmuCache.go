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
type State struct {
	CurrentState           string               `json:"current_state"`
	Table                  []internal.SetState   `json:"table"`
	InflightFlushReqID     string               `json:"inflight_flush_req_id"`
	InflightFlushReqSrc    sim.RemotePort       `json:"inflight_flush_req_src"`
	InflightFlushReqActive bool                 `json:"inflight_flush_req_active"`
}

// Comp is the mmuCache component.
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

// SyncToState exports the runtime mutable data into the State struct.
func (c *Comp) SyncToState() {
	s := State{
		CurrentState: c.state,
		Table:        make([]internal.SetState, len(c.table)),
	}

	for i, set := range c.table {
		s.Table[i] = set.ExportState()
	}

	if c.inflightFlushReq != nil {
		s.InflightFlushReqID = c.inflightFlushReq.ID
		s.InflightFlushReqSrc = c.inflightFlushReq.Src
		s.InflightFlushReqActive = true
	}

	c.SetState(s)
}

// SyncFromState restores the runtime mutable data from the State struct.
func (c *Comp) SyncFromState() {
	s := c.GetState()

	c.state = s.CurrentState

	spec := c.GetSpec()
	c.table = make([]internal.Set, len(s.Table))
	for i, ss := range s.Table {
		set := internal.NewSet(spec.NumBlocks)
		set.ImportState(ss)
		c.table[i] = set
	}

	if s.InflightFlushReqActive {
		c.inflightFlushReq = &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:  s.InflightFlushReqID,
				Src: s.InflightFlushReqSrc,
			},
		}
	} else {
		c.inflightFlushReq = nil
	}
}
