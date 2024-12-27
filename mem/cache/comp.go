package cache

import (
	"reflect"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/simulation"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&transaction{}))
	serialization.RegisterType(reflect.TypeOf(&state{}))
}

type transactionType int

const (
	transactionTypeReadHit transactionType = iota
	transactionTypeReadMiss
	transactionTypeReadMSHRHit
	transactionTypeWriteHit
	transactionTypeWriteMiss
	transactionTypeWriteMSHRHit
)

type transaction struct {
	transType    transactionType
	req          mem.AccessReq
	reqToBottom  mem.AccessReq
	setID, wayID int
}

func (t *transaction) TaskID() string {
	return "cache-trans-" + t.req.Meta().ID
}

func (t *transaction) Name() string {
	return "cache-trans-" + t.req.Meta().ID
}

func (t *transaction) Serialize() (map[string]any, error) {
	return map[string]any{
		"req":   t.req,
		"setID": t.setID,
		"wayID": t.wayID,
	}, nil
}

func (t *transaction) Deserialize(data map[string]any) error {
	t.req = data["req"].(mem.AccessReq)
	t.setID = int(data["setID"].(uint64))
	t.wayID = int(data["wayID"].(uint64))

	return nil
}

type state struct {
	name         string
	Transactions []*transaction
}

func (s *state) Name() string {
	return s.name
}

func (s *state) Serialize() (map[string]any, error) {
	return map[string]any{
		"transactions": s.Transactions,
	}, nil
}

func (s *state) Deserialize(data map[string]any) error {
	s.Transactions = data["transactions"].([]*transaction)

	return nil
}

// A Comp implements a cache.
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder
	*state

	topPort    modeling.Port
	bottomPort modeling.Port

	NumReqPerCycle int
	Log2BlockSize  int

	MSHR                     mshr.MSHR
	Tags                     tagging.Tags
	VictimFinder             tagging.VictimFinder
	Storage                  *mem.Storage
	AddressToDstTable        mem.AddressToPortMapper
	EvictQueue               queueing.Buffer
	TopDownPreStorageBuffer  queueing.Buffer
	BottomUpPreStorageBuffer queueing.Buffer
	PostStorageBuffer        queueing.Buffer
	StoragePipeline          queueing.Pipeline
}

func (c *Comp) State() simulation.State {
	return c.state
}

// Tick updates the state of the cache.
func (c *Comp) Tick() bool {
	c.MiddlewareHolder.Tick()

	return true
}
