package mmuCache

import (
	"github.com/sarchlab/akita/v4/mem/vm/mmuCache/internal"
	"github.com/sarchlab/akita/v4/sim"
)

type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	UpModule  sim.Port
	LowModule sim.Port
	state     string

	numBlocks       int
	numLevels       int
	pageSize        uint64
	log2PageSize    uint64
	numReqPerCycle  int
	latencyPerLevel uint64

	table []internal.Set

	inflightFlushReq *FlushReq
}

func (c *Comp) reset() {
	c.table = make([]internal.Set, c.numLevels)
	for i := 0; i < c.numLevels; i++ {
		c.table[i] = internal.NewSet(c.numBlocks)
	}
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}
