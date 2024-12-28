package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
)

// A storageMiddleware handles the transactions going to the storage of the
// cache.
type storageMiddleware struct {
	*Comp
}

func (m *storageMiddleware) Tick() (madeProgress bool) {
	for range m.numReqPerCycle {
		madeProgress = m.generateRspFromMSHR() || madeProgress
	}

	for range m.numReqPerCycle {
		madeProgress = m.processPostPipelineBuffer() || madeProgress
	}

	madeProgress = m.storagePipeline.Tick() || madeProgress

	for range m.numReqPerCycle {
		madeProgress = m.processPrePipelineBuffer() || madeProgress
	}

	return
}

func (m *storageMiddleware) generateRspFromMSHR() bool {
	return false
}

func (m *storageMiddleware) processPostPipelineBuffer() bool {
	item := m.storagePostPipelineBuf.Peek()
	if item == nil {
		return false
	}

	trans := item.(*transaction)
	switch trans.transType {
	case transactionTypeReadHit:
		return m.processReadHit(trans)
	default:
		panic("unknown transaction type")
	}
}

func (m *storageMiddleware) processReadHit(trans *transaction) bool {
	cacheAddr := trans.block.CacheAddress

	data, err := m.storage.Read(cacheAddr, 1<<m.log2BlockSize)
	if err != nil {
		panic(err)
	}

	cacheLineAddr := getCacheLineAddr(trans.req.GetAddress(), m.log2BlockSize)
	offset := trans.req.GetAddress() - cacheLineAddr
	data = data[offset : offset+trans.req.GetByteSize()]

	rsp := mem.DataReadyRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.topPort.AsRemote(),
			Dst: trans.req.Meta().Src,
		},
		Data: data,
	}

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	m.storagePostPipelineBuf.Pop()
	m.removeTransaction(trans)
	m.tags.Unlock(trans.block.SetID, trans.block.WayID)

	m.traceReqEnd(trans.req)

	return true
}

func (m *storageMiddleware) processPrePipelineBuffer() bool {
	if !m.storagePipeline.CanAccept() {
		return false
	}

	item := m.storageBottomUpBuf.Pop()
	if item != nil {
		m.storagePipeline.Accept(item.(queueing.PipelineItem))
		return true
	}

	item = m.storageTopDownBuf.Pop()
	if item != nil {
		m.storagePipeline.Accept(item.(queueing.PipelineItem))
		return true
	}

	return false
}
