package cache

import (
	"fmt"

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
	trans := m.state.RespondingTrans
	if trans == nil {
		return false
	}

	pid := trans.req.GetPID()
	addr := trans.req.GetAddress()

	if !m.mshr.Lookup(pid, addr) {
		panic(fmt.Sprintf("MSHR does not have entry for 0x%016x",
			trans.req.GetAddress()))
	}

	nextReq, err := m.mshr.GetNextReqInEntry(pid, addr)
	if err != nil {
		m.state.RespondingTrans = nil
		m.mshr.RemoveEntry(pid, addr)

		return true
	}

	data := make([]byte, nextReq.GetByteSize())

	rsp := mem.DataReadyRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.topPort.AsRemote(),
			Dst: nextReq.Meta().Src,
		},
		Data:      data,
		RespondTo: nextReq.Meta().ID,
	}

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	_ = m.mshr.RemoveReqFromEntry(nextReq.Meta().ID)

	return true
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
	case transactionTypeReadMiss:
		return m.processReadMiss(trans)
	default:
		panic(fmt.Sprintf("unknown transaction type: %s",
			trans.transType.String()))
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

func (m *storageMiddleware) processReadMiss(trans *transaction) bool {
	m.storage.Write(
		trans.block.CacheAddress,
		trans.rspFromBottom.(mem.DataReadyRsp).Data,
	)

	m.state.RespondingTrans = trans
	m.storagePostPipelineBuf.Pop()

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
