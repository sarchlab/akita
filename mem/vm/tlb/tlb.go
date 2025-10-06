package tlb

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
)

const (
	tlbStateEnable = "enable"
	tlbStatePause  = "pause"
	tlbStateDrain  = "drain"
	tlbStateFlush  = "flush"
)

// Comp is a Translation Lookaside Buffer (TLB) that stores part of the page
// table.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	addressMapper mem.AddressToPortMapper

	numSets        int
	numWays        int
	pageSize       uint64
	numReqPerCycle int
	state          string

	sets []internal.Set

	mshr                mshr
	respondingMSHREntry *mshrEntry
	responsePipeline    pipelining.Pipeline
	responseBuffer      sim.Buffer

	inflightFlushReq *FlushReq
}

// reset sets all the entries in the TLB to be invalid
func (c *Comp) reset() {
	c.sets = make([]internal.Set, c.numSets)
	for i := 0; i < c.numSets; i++ {
		set := internal.NewSet(c.numWays)
		c.sets[i] = set
	}
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}
