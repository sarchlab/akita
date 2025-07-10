package writeback

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	cachemiddlewares "github.com/sarchlab/akita/v4/mem/cache/cacheMiddlewares"
	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"
)

type cacheState int

const (
	cacheStateInvalid cacheState = iota
	cacheStateRunning
	cacheStatePreFlushing
	cacheStateFlushing
	cacheStatePaused
)

// Comp in the writeback package is a cache that performs the write-back policy.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	dirStageBuffer           sim.Buffer
	dirToBankBuffers         []sim.Buffer
	writeBufferToBankBuffers []sim.Buffer
	mshrStageBuffer          sim.Buffer
	writeBufferBuffer        sim.Buffer

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
	flusher     *flusher

	storage             *mem.Storage
	addressToPortMapper mem.AddressToPortMapper
	directory           cache.Directory
	mshr                cache.MSHR
	log2BlockSize       uint64
	numReqPerCycle      int

	state                cacheState
	inFlightTransactions []*transaction
	evictingList         map[uint64]bool
}

// SetAddressToPortMapper sets the AddressToPortMapper used by the cache.
func (c *Comp) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	c.addressToPortMapper = lmf
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates the internal states of the Cache.
func (m *middleware) Tick() bool {
	madeProgress := false

	if m.state != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	madeProgress = m.flusher.Tick() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	madeProgress = m.runStage(m.mshrStage) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer) || madeProgress
	madeProgress = m.runStage(m.dirStage) || madeProgress
	madeProgress = m.runStage(m.topParser) || madeProgress

	return madeProgress
}

func (m *middleware) runStage(stage sim.Ticker) bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = stage.Tick() || madeProgress
	}

	return madeProgress
}

func (c *Comp) discardInflightTransactions() {
	sets := c.directory.GetSets()
	for _, set := range sets {
		for _, block := range set.Blocks {
			block.ReadCount = 0
			block.IsLocked = false
		}
	}

	c.dirStage.Reset()

	for _, bs := range c.bankStages {
		bs.Reset()
	}

	c.mshrStage.Reset()
	c.writeBuffer.Reset()

	clearPort(c.topPort)

	// for _, t := range c.inFlightTransactions {
	// 	fmt.Printf("%.10f, %s, transaction %s discarded due to flushing\n",
	// 		now, c.Name(), t.id)
	// }

	c.inFlightTransactions = nil
}

// CoalescerComp interface implementation for writeback cache
func (c *Comp) TopPort() sim.Port {
	return c.topPort
}

func (c *Comp) DirBufCanPush() bool {
	return c.dirStageBuffer.CanPush()
}

func (c *Comp) DirBufPush(item interface{}) {
	c.dirStageBuffer.Push(item)
}

func (c *Comp) AddTransaction(txn cachemiddlewares.Transaction) {
	if t, ok := txn.(*transaction); ok {
		c.inFlightTransactions = append(c.inFlightTransactions, t)
	}
}

func (c *Comp) PostCoalesce(txn cachemiddlewares.Transaction) {
	if t, ok := txn.(*transaction); ok {
		c.dirStageBuffer.Push(t)
	}
}

func (c *Comp) Log2BlockSize() int {
	return int(c.log2BlockSize)
}

func (c *Comp) Transactions() []cachemiddlewares.Transaction {
	result := make([]cachemiddlewares.Transaction, len(c.inFlightTransactions))
	for i, t := range c.inFlightTransactions {
		result[i] = t
	}
	return result
}

func (c *Comp) MaxNumConcurrentTrans() int {
	// For writeback cache, we can estimate based on MSHR size
	// This is a reasonable default, but can be adjusted based on actual implementation
	return 64 // or could be based on c.mshr.MaxSize() if available
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
			id:   sim.GetIDGenerator().Generate(),
			read: coalescedRead,
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
			id:    sim.GetIDGenerator().Generate(),
			write: write,
		}
	}
}
