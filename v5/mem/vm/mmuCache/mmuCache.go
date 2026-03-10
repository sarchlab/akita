package mmuCache

import (
	"io"

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

	inflightFlushReq *sim.GenericMsg // payload: *FlushReqPayload
}

func (c *Comp) reset() {
	spec := c.GetSpec()
	c.table = make([]internal.Set, spec.NumLevels)
	for i := 0; i < spec.NumLevels; i++ {
		c.table[i] = internal.NewSet(spec.NumBlocks)
	}
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
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

	c.Component.SetState(s)

	return s
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)

	c.state = state.CurrentState

	spec := c.GetSpec()
	c.table = make([]internal.Set, len(state.Table))
	for i, ss := range state.Table {
		set := internal.NewSet(spec.NumBlocks)
		set.ImportState(ss)
		c.table[i] = set
	}

	if state.InflightFlushReqActive {
		c.inflightFlushReq = &sim.GenericMsg{
			MsgMeta: sim.MsgMeta{
				ID:  state.InflightFlushReqID,
				Src: state.InflightFlushReqSrc,
			},
		}
	} else {
		c.inflightFlushReq = nil
	}
}

// SaveState syncs runtime data to state, then delegates to Component.SaveState.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState loads state from the reader, then restores runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}

	c.SetState(c.Component.GetState())

	return nil
}
