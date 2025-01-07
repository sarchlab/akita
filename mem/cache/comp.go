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
	transactionTypeInvalid transactionType = iota
	transactionTypeReadHit
	transactionTypeReadMiss
	transactionTypeReadMSHRHit
	transactionTypeWriteHit
	transactionTypeWriteMiss
	transactionTypeWriteMSHRHit
)

func (t transactionType) String() string {
	return []string{
		"invalid",
		"readHit",
		"readMiss",
		"readMSHRHit",
		"writeHit",
		"writeMiss",
		"writeMSHRHit",
	}[t]
}

type transaction struct {
	transType     transactionType
	req           mem.AccessReq
	reqToBottom   mem.AccessReq
	rspFromBottom mem.AccessRsp
	block         tagging.Block
}

func (t *transaction) TaskID() string {
	return "cache-trans-" + t.req.Meta().ID
}

func (t *transaction) Name() string {
	return "cache-trans-" + t.req.Meta().ID
}

func (t *transaction) Serialize() (map[string]any, error) {
	return map[string]any{
		"transType":   t.transType,
		"req":         t.req,
		"reqToBottom": t.reqToBottom,
		"block":       t.block,
	}, nil
}

func (t *transaction) Deserialize(data map[string]any) error {
	t.transType = transactionType(data["transType"].(int))
	t.req = data["req"].(mem.AccessReq)
	t.reqToBottom = data["reqToBottom"].(mem.AccessReq)
	t.block = data["block"].(tagging.Block)

	return nil
}

type state struct {
	name            string
	Transactions    []*transaction
	RespondingTrans *transaction
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

	numReqPerCycle int
	log2BlockSize  int

	mshr                   mshr.MSHR
	tags                   tagging.TagArray
	victimFinder           tagging.VictimFinder
	storage                *mem.Storage
	addressToDstTable      mem.AddressToPortMapper
	storageTopDownBuf      queueing.Buffer
	storageBottomUpBuf     queueing.Buffer
	bottomInteractionBuf   queueing.Buffer
	storagePostPipelineBuf queueing.Buffer
	storagePipeline        queueing.Pipeline
}

func (c *Comp) State() simulation.State {
	return c.state
}

// Tick updates the state of the cache.
func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

func (c *Comp) findTransByReqToBottomID(reqID string) (*transaction, bool) {
	for _, trans := range c.state.Transactions {
		if trans.reqToBottom.Meta().ID == reqID {
			return trans, true
		}
	}

	return nil, false
}

func (c *Comp) removeTransaction(trans *transaction) {
	for i, t := range c.state.Transactions {
		if t == trans {
			c.state.Transactions = append(
				c.state.Transactions[:i], c.state.Transactions[i+1:]...)
			break
		}
	}
}

func getCacheLineAddr(addr uint64, log2BlockSize int) uint64 {
	return addr & ^((uint64(1) << log2BlockSize) - 1)
}
