package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
)

type transaction struct {
	req mem.AccessReq

	setID, wayID int
}

type state struct {
	NumReqPerCycle int
	Log2BlockSize  int

	MSHR              mshr.MSHR
	Tags              tagging.Tags
	VictimFinder      tagging.VictimFinder
	Storage           *mem.Storage
	AddressToDstTable mem.AddressToPortMapper

	Transactions []transaction

	EvictQueue               queueing.Buffer
	TopDownPreStorageBuffer  queueing.Buffer
	BottomUpPreStorageBuffer queueing.Buffer
	PostStorageBuffer        queueing.Buffer
	StoragePipeline          queueing.Pipeline
}

// A Comp implements a cache.
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	topPort    modeling.Port
	bottomPort modeling.Port

	state
}

// Tick updates the state of the cache.
func (c *Comp) Tick() bool {
	c.MiddlewareHolder.Tick()

	return true
}
