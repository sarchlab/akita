package simplecache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"github.com/sarchlab/akita/v5/tracing"
)

type coalescer struct {
	cache      *pipelineMW
	toCoalesce []*transactionState
}

func (c *coalescer) Reset() {
	c.toCoalesce = nil
}

func (c *coalescer) Tick() bool {
	msgI := c.cache.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	return c.processReq(msgI)
}

func (c *coalescer) getDirBuf() *stateutil.Buffer[int] {
	next := c.cache.comp.GetNextState()
	return &next.DirBuf
}

func (c *coalescer) processReq(msg sim.Msg) bool {
	if len(c.cache.transactions) >= c.cache.GetSpec().MaxNumConcurrentTrans {
		return false
	}

	if c.isReqLastInWave(msg) {
		if len(c.toCoalesce) == 0 || c.canReqCoalesce(msg) {
			return c.processReqLastInWaveCoalescable(msg)
		}

		return c.processReqLastInWaveNoncoalescable(msg)
	}

	if len(c.toCoalesce) == 0 || c.canReqCoalesce(msg) {
		return c.processReqCoalescable(msg)
	}

	return c.processReqNoncoalescable(msg)
}

func (c *coalescer) processReqCoalescable(msg sim.Msg) bool {
	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

func (c *coalescer) processReqNoncoalescable(msg sim.Msg) bool {
	dirBuf := c.getDirBuf()
	if !dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend()

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(msg sim.Msg) bool {
	dirBuf := c.getDirBuf()
	if !dirBuf.CanPush() {
		return false
	}

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend()
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

func (c *coalescer) processReqLastInWaveNoncoalescable(msg sim.Msg) bool {
	dirBuf := c.getDirBuf()
	if !dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend()

	if !dirBuf.CanPush() {
		return true
	}

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend()
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

func (c *coalescer) createTransaction(msg sim.Msg) *transactionState {
	switch m := msg.(type) {
	case *mem.ReadReq:
		t := &transactionState{
			read: m,
		}

		return t
	case *mem.WriteReq:
		t := &transactionState{
			write: m,
		}

		return t
	default:
		log.Panicf("cannot process request of type %s\n",
			reflect.TypeOf(msg))
		return nil
	}
}

func (c *coalescer) isReqLastInWave(msg sim.Msg) bool {
	switch m := msg.(type) {
	case *mem.ReadReq:
		return !m.CanWaitForCoalesce
	case *mem.WriteReq:
		return !m.CanWaitForCoalesce
	default:
		panic("unknown type")
	}
}

func (c *coalescer) canReqCoalesce(msg sim.Msg) bool {
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	accessReq := msg.(mem.AccessReq)
	return accessReq.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize
}

func (c *coalescer) coalesceAndSend() bool {
	var trans *transactionState
	if c.toCoalesce[0].read != nil {
		trans = c.coalesceRead()
		tracing.StartTaskWithSpecificLocation(trans.id,
			tracing.MsgIDAtReceiver(c.toCoalesce[0].read, c.cache.comp),
			c.cache.comp, "cache_transaction", "read",
			c.cache.comp.Name()+".Local",
			nil)
	} else {
		trans = c.coalesceWrite()
		tracing.StartTaskWithSpecificLocation(trans.id,
			tracing.MsgIDAtReceiver(c.toCoalesce[0].write, c.cache.comp),
			c.cache.comp, "cache_transaction", "write",
			c.cache.comp.Name()+".Local",
			nil)
	}

	// Add to postCoalesceTransactions BEFORE pushing to buffer
	c.cache.postCoalesceTransactions =
		append(c.cache.postCoalesceTransactions, trans)

	// Push the index into the DirBuf
	idx := len(c.cache.postCoalesceTransactions) - 1
	dirBuf := c.getDirBuf()
	dirBuf.PushTyped(idx)

	c.toCoalesce = nil

	return true
}

func (c *coalescer) coalesceRead() *transactionState {
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	coalescedRead := &mem.ReadReq{}
	coalescedRead.ID = sim.GetIDGenerator().Generate()
	coalescedRead.Address = cachelineID
	coalescedRead.AccessByteSize = blockSize
	coalescedRead.PID = c.toCoalesce[0].PID()
	coalescedRead.TrafficBytes = 12
	coalescedRead.TrafficClass = "req"

	return &transactionState{
		id:                      sim.GetIDGenerator().Generate(),
		read:                    coalescedRead,
		preCoalesceTransactions: c.toCoalesce,
	}
}

func (c *coalescer) coalesceWrite() *transactionState {
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	writeData := make([]byte, blockSize)
	writeDirtyMask := make([]bool, blockSize)
	coalescedWrite := &mem.WriteReq{}
	coalescedWrite.ID = sim.GetIDGenerator().Generate()
	coalescedWrite.Address = cachelineID
	coalescedWrite.PID = c.toCoalesce[0].PID()
	coalescedWrite.Data = writeData
	coalescedWrite.DirtyMask = writeDirtyMask
	coalescedWrite.TrafficBytes = len(writeData) + 12
	coalescedWrite.TrafficClass = "req"

	for _, t := range c.toCoalesce {
		offset := int(t.write.Address - cachelineID)

		for i := 0; i < len(t.write.Data); i++ {
			if t.write.DirtyMask == nil || t.write.DirtyMask[i] {
				coalescedWrite.Data[i+offset] = t.write.Data[i]
				coalescedWrite.DirtyMask[i+offset] = true
			}
		}
	}

	return &transactionState{
		id:                      sim.GetIDGenerator().Generate(),
		write:                   coalescedWrite,
		preCoalesceTransactions: c.toCoalesce,
	}
}
