package writethrough

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	cachemiddlewares "github.com/sarchlab/akita/v4/mem/cache/cacheMiddlewares"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// Comp is a customized L1 cache the for R9nano GPUs.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	numReqPerCycle      int
	log2BlockSize       uint64
	storage             *mem.Storage
	directory           cache.Directory
	mshr                cache.MSHR
	bankLatency         int
	wayAssociativity    int
	addressToPortMapper mem.AddressToPortMapper

	dirBuf   sim.Buffer
	bankBufs []sim.Buffer

	coalesceStage    *coalescer
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
	controlStage     *controlStage

	maxNumConcurrentTrans    int
	transactions             []*transaction
	postCoalesceTransactions []*transaction

	isPaused bool
}

// SetAddressToPortMapper sets the finder that tells which remote port can serve
// the data on a certain address.
func (c *Comp) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	c.addressToPortMapper = lmf
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick update the state of the cache
func (m *middleware) Tick() bool {
	madeProgress := false

	if !m.isPaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.controlStage.Tick() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false
	madeProgress = m.tickRespondStage() || madeProgress
	madeProgress = m.tickParseBottomStage() || madeProgress
	madeProgress = m.tickBankStage() || madeProgress
	madeProgress = m.tickDirectoryStage() || madeProgress
	madeProgress = m.tickCoalesceState() || madeProgress

	return madeProgress
}

func (m *middleware) tickRespondStage() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickParseBottomStage() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseBottomStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickBankStage() bool {
	madeProgress := false
	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickDirectoryStage() bool {
	return m.directoryStage.Tick()
}

func (m *middleware) tickCoalesceState() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.coalesceStage.Tick() || madeProgress
	}

	return madeProgress
}

// CoalescerComp interface implementation for writethrough cache
func (c *Comp) TopPort() sim.Port {
	return c.topPort
}

func (c *Comp) DirBufCanPush() bool {
	return c.dirBuf.CanPush()
}

func (c *Comp) DirBufPush(item interface{}) {
	c.dirBuf.Push(item)
}

func (c *Comp) AddTransaction(txn cachemiddlewares.Transaction) {
	if t, ok := txn.(*transaction); ok {
		c.transactions = append(c.transactions, t)
	}
}

func (c *Comp) PostCoalesce(txn cachemiddlewares.Transaction) {
	if t, ok := txn.(*transaction); ok {
		c.dirBuf.Push(t)
		c.postCoalesceTransactions = append(c.postCoalesceTransactions, t)
	}
}

func (c *Comp) Log2BlockSize() int {
	return int(c.log2BlockSize)
}

func (c *Comp) Transactions() []cachemiddlewares.Transaction {
	result := make([]cachemiddlewares.Transaction, len(c.transactions))
	for i, t := range c.transactions {
		result[i] = t
	}
	return result
}

func (c *Comp) MaxNumConcurrentTrans() int {
	return c.maxNumConcurrentTrans
}

func (c *Comp) CreateTransaction(req mem.AccessReq) cachemiddlewares.Transaction {
	switch req := req.(type) {
	case *mem.ReadReq:
		return &transaction{
			id:   sim.GetIDGenerator().Generate(),
			read: req,
		}
	case *mem.WriteReq:
		return &transaction{
			id:    sim.GetIDGenerator().Generate(),
			write: req,
		}
	default:
		return nil
	}
}

func (c *Comp) CreateCoalescedTransaction(transactions []cachemiddlewares.Transaction) cachemiddlewares.Transaction {
	// Convert to our transaction type
	txns := make([]*transaction, len(transactions))
	for i, t := range transactions {
		if tx, ok := t.(*transaction); ok {
			txns[i] = tx
		}
	}

	if len(txns) == 0 {
		return nil
	}

	blockSize := uint64(1 << c.log2BlockSize)
	cachelineID := txns[0].Address() / blockSize * blockSize

	if txns[0].IsRead() {
		coalescedRead := mem.ReadReqBuilder{}.
			WithAddress(cachelineID).
			WithByteSize(blockSize).
			WithPID(txns[0].PID()).
			Build()

		return &transaction{
			id:                      sim.GetIDGenerator().Generate(),
			read:                    coalescedRead,
			preCoalesceTransactions: txns,
		}
	} else {
		write := mem.WriteReqBuilder{}.
			WithAddress(cachelineID).
			WithPID(txns[0].PID()).
			WithData(make([]byte, blockSize)).
			WithDirtyMask(make([]bool, blockSize)).
			Build()

		for _, t := range txns {
			w := t.write
			offset := int(w.Address - cachelineID)

			for i := 0; i < len(w.Data); i++ {
				if w.DirtyMask == nil || w.DirtyMask[i] {
					write.Data[i+offset] = w.Data[i]
					write.DirtyMask[i+offset] = true
				}
			}
		}

		return &transaction{
			id:                      sim.GetIDGenerator().Generate(),
			write:                   write,
			preCoalesceTransactions: txns,
		}
	}
}
