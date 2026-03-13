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
	toCoalesce []int // absolute indices into State.Transactions
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
	next := c.cache.comp.GetNextState()
	if next.NumTransactions >= c.cache.GetSpec().MaxNumConcurrentTrans {
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
	transIdx := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, transIdx)
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

	transIdx := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, transIdx)
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(msg sim.Msg) bool {
	dirBuf := c.getDirBuf()
	if !dirBuf.CanPush() {
		return false
	}

	transIdx := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, transIdx)
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

	transIdx := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, transIdx)
	c.coalesceAndSend()
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache.comp)

	return true
}

// createTransaction creates a new pre-coalesce transaction in State and
// returns its absolute index in State.Transactions.
func (c *coalescer) createTransaction(msg sim.Msg) int {
	next := c.cache.comp.GetNextState()

	var t transactionState
	switch m := msg.(type) {
	case *mem.ReadReq:
		t = transactionState{
			HasRead:            true,
			ReadMeta:           m.MsgMeta,
			ReadAddress:        m.Address,
			ReadAccessByteSize: m.AccessByteSize,
			ReadPID:            m.PID,
		}
	case *mem.WriteReq:
		t = transactionState{
			HasWrite:       true,
			WriteMeta:      m.MsgMeta,
			WriteAddress:   m.Address,
			WriteData:      m.Data,
			WriteDirtyMask: m.DirtyMask,
			WritePID:       m.PID,
		}
	default:
		log.Panicf("cannot process request of type %s\n",
			reflect.TypeOf(msg))
		return -1
	}

	return next.addPreCoalesceTrans(t)
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
	next := c.cache.comp.GetNextState()
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	accessReq := msg.(mem.AccessReq)
	firstTrans := &next.Transactions[c.toCoalesce[0]]
	return accessReq.GetAddress()/blockSize == firstTrans.Address()/blockSize
}

func (c *coalescer) coalesceAndSend() bool {
	next := c.cache.comp.GetNextState()
	firstTrans := &next.Transactions[c.toCoalesce[0]]

	var postCoalesceTrans transactionState
	if firstTrans.HasRead {
		postCoalesceTrans = c.coalesceRead()
		tracing.StartTaskWithSpecificLocation(postCoalesceTrans.ID,
			tracing.MsgIDAtReceiver(
				c.readReqFromTrans(firstTrans), c.cache.comp),
			c.cache.comp, "cache_transaction", "read",
			c.cache.comp.Name()+".Local",
			nil)
	} else {
		postCoalesceTrans = c.coalesceWrite()
		tracing.StartTaskWithSpecificLocation(postCoalesceTrans.ID,
			tracing.MsgIDAtReceiver(
				c.writeReqFromTrans(firstTrans), c.cache.comp),
			c.cache.comp, "cache_transaction", "write",
			c.cache.comp.Name()+".Local",
			nil)
	}

	// Append to post-coalesce section (end of Transactions)
	postIdx := next.addPostCoalesceTrans(postCoalesceTrans)

	// Push the post-coalesce index into the DirBuf
	dirBuf := c.getDirBuf()
	dirBuf.PushTyped(postIdx)

	c.toCoalesce = nil

	return true
}

// readReqFromTrans reconstructs a *mem.ReadReq at a send/trace boundary.
func (c *coalescer) readReqFromTrans(t *transactionState) *mem.ReadReq {
	return &mem.ReadReq{
		MsgMeta:        t.ReadMeta,
		Address:        t.ReadAddress,
		AccessByteSize: t.ReadAccessByteSize,
		PID:            t.ReadPID,
	}
}

// writeReqFromTrans reconstructs a *mem.WriteReq at a send/trace boundary.
func (c *coalescer) writeReqFromTrans(t *transactionState) *mem.WriteReq {
	return &mem.WriteReq{
		MsgMeta:   t.WriteMeta,
		Address:   t.WriteAddress,
		Data:      t.WriteData,
		DirtyMask: t.WriteDirtyMask,
		PID:       t.WritePID,
	}
}

func (c *coalescer) coalesceRead() transactionState {
	next := c.cache.comp.GetNextState()
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	firstTrans := &next.Transactions[c.toCoalesce[0]]
	cachelineID := firstTrans.Address() / blockSize * blockSize

	readID := sim.GetIDGenerator().Generate()
	readMeta := sim.MsgMeta{
		ID:           readID,
		TrafficBytes: 12,
		TrafficClass: "req",
	}

	return transactionState{
		ID:                   sim.GetIDGenerator().Generate(),
		HasRead:              true,
		ReadMeta:             readMeta,
		ReadAddress:          cachelineID,
		ReadAccessByteSize:   blockSize,
		ReadPID:              firstTrans.PID(),
		PreCoalesceTransIdxs: append([]int(nil), c.toCoalesce...),
	}
}

func (c *coalescer) coalesceWrite() transactionState {
	next := c.cache.comp.GetNextState()
	blockSize := uint64(1 << c.cache.GetSpec().Log2BlockSize)
	firstTrans := &next.Transactions[c.toCoalesce[0]]
	cachelineID := firstTrans.Address() / blockSize * blockSize
	writeData := make([]byte, blockSize)
	writeDirtyMask := make([]bool, blockSize)

	writeID := sim.GetIDGenerator().Generate()
	writeMeta := sim.MsgMeta{
		ID:           writeID,
		TrafficBytes: len(writeData) + 12,
		TrafficClass: "req",
	}

	for _, tIdx := range c.toCoalesce {
		t := &next.Transactions[tIdx]
		offset := int(t.WriteAddress - cachelineID)

		for i := 0; i < len(t.WriteData); i++ {
			if t.WriteDirtyMask == nil || t.WriteDirtyMask[i] {
				writeData[i+offset] = t.WriteData[i]
				writeDirtyMask[i+offset] = true
			}
		}
	}

	return transactionState{
		ID:                   sim.GetIDGenerator().Generate(),
		HasWrite:             true,
		WriteMeta:            writeMeta,
		WriteAddress:         cachelineID,
		WriteData:            writeData,
		WriteDirtyMask:       writeDirtyMask,
		WritePID:             firstTrans.PID(),
		PreCoalesceTransIdxs: append([]int(nil), c.toCoalesce...),
	}
}
