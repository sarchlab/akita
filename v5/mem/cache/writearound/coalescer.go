package writearound

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type coalescer struct {
	cache      *Comp
	toCoalesce []*transaction
}

func (c *coalescer) Reset() {
	c.toCoalesce = nil
}

func (c *coalescer) Tick() bool {
	msgI := c.cache.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	msg := msgI.(*sim.GenericMsg)
	return c.processReq(msg)
}

func (c *coalescer) processReq(msg *sim.GenericMsg) bool {
	if len(c.cache.transactions) >= c.cache.maxNumConcurrentTrans {
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

func (c *coalescer) processReqCoalescable(msg *sim.GenericMsg) bool {
	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache)

	return true
}

func (c *coalescer) processReqNoncoalescable(msg *sim.GenericMsg) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend()

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache)

	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(msg *sim.GenericMsg) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend()
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache)

	return true
}

func (c *coalescer) processReqLastInWaveNoncoalescable(msg *sim.GenericMsg) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend()

	if !c.cache.dirBuf.CanPush() {
		return true
	}

	trans := c.createTransaction(msg)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend()
	c.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, c.cache)

	return true
}

func (c *coalescer) createTransaction(msg *sim.GenericMsg) *transaction {
	switch msg.Payload.(type) {
	case *mem.ReadReqPayload:
		t := &transaction{
			read: msg,
		}

		return t
	case *mem.WriteReqPayload:
		t := &transaction{
			write: msg,
		}

		return t
	default:
		log.Panicf("cannot process request of type %s\n",
			reflect.TypeOf(msg.Payload))
		return nil
	}
}

func (c *coalescer) isReqLastInWave(msg *sim.GenericMsg) bool {
	switch payload := msg.Payload.(type) {
	case *mem.ReadReqPayload:
		return !payload.CanWaitForCoalesce
	case *mem.WriteReqPayload:
		return !payload.CanWaitForCoalesce
	default:
		panic("unknown type")
	}
}

func (c *coalescer) canReqCoalesce(msg *sim.GenericMsg) bool {
	blockSize := uint64(1 << c.cache.log2BlockSize)
	payload := msg.Payload.(mem.AccessReqPayload)
	return payload.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize
}

func (c *coalescer) coalesceAndSend() bool {
	var trans *transaction
	if c.toCoalesce[0].read != nil {
		trans = c.coalesceRead()
		tracing.StartTaskWithSpecificLocation(trans.id,
			tracing.MsgIDAtReceiver(c.toCoalesce[0].read, c.cache),
			c.cache, "cache_transaction", "read",
			c.cache.Name()+".Local",
			nil)
	} else {
		trans = c.coalesceWrite()
		tracing.StartTaskWithSpecificLocation(trans.id,
			tracing.MsgIDAtReceiver(c.toCoalesce[0].write, c.cache),
			c.cache, "cache_transaction", "write",
			c.cache.Name()+".Local",
			nil)
	}

	c.cache.dirBuf.Push(trans)
	c.cache.postCoalesceTransactions =
		append(c.cache.postCoalesceTransactions, trans)
	c.toCoalesce = nil

	return true
}

func (c *coalescer) coalesceRead() *transaction {
	blockSize := uint64(1 << c.cache.log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	coalescedRead := mem.ReadReqBuilder{}.
		WithAddress(cachelineID).
		WithByteSize(blockSize).
		WithPID(c.toCoalesce[0].PID()).
		Build()

	return &transaction{
		id:                      sim.GetIDGenerator().Generate(),
		read:                    coalescedRead,
		preCoalesceTransactions: c.toCoalesce,
	}
}

func (c *coalescer) coalesceWrite() *transaction {
	blockSize := uint64(1 << c.cache.log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	coalescedWrite := mem.WriteReqBuilder{}.
		WithAddress(cachelineID).
		WithPID(c.toCoalesce[0].PID()).
		WithData(make([]byte, blockSize)).
		WithDirtyMask(make([]bool, blockSize)).
		Build()

	coalescedPayload := sim.MsgPayload[mem.WriteReqPayload](coalescedWrite)

	for _, t := range c.toCoalesce {
		wPayload := sim.MsgPayload[mem.WriteReqPayload](t.write)
		offset := int(wPayload.Address - cachelineID)

		for i := 0; i < len(wPayload.Data); i++ {
			if wPayload.DirtyMask == nil || wPayload.DirtyMask[i] {
				coalescedPayload.Data[i+offset] = wPayload.Data[i]
				coalescedPayload.DirtyMask[i+offset] = true
			}
		}
	}

	return &transaction{
		id:                      sim.GetIDGenerator().Generate(),
		write:                   coalescedWrite,
		preCoalesceTransactions: c.toCoalesce,
	}
}
