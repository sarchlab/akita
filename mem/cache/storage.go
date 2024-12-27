package cache

import (
	"github.com/sarchlab/akita/v4/sim/queueing"
)

// A storageMiddleware handles the transactions going to the storage of the
// cache.
type storageMiddleware struct {
	*Comp
}

func (m *storageMiddleware) Tick() (madeProgress bool) {
	for range m.NumReqPerCycle {
		madeProgress = m.processPostStorageBuffer() || madeProgress
	}

	for range m.NumReqPerCycle {
		madeProgress = m.StoragePipeline.Tick() || madeProgress
	}

	for range m.NumReqPerCycle {
		madeProgress = m.processPreStorageBuffer() || madeProgress
	}

	return
}

func (m *storageMiddleware) processPostStorageBuffer() bool {
	item := m.PostStorageBuffer.Peek()
	if item == nil {
		return false
	}

	transaction := item.(*transaction)

	switch transaction.transType {
	case transactionTypeReadHit:
		m.StoragePipeline.Accept(transaction)
	default:
		panic("unsupported transaction type")
	}

	return true
}

func (m *storageMiddleware) processPreStorageBuffer() bool {
	if !m.StoragePipeline.CanAccept() {
		return false
	}

	item := m.BottomUpPreStorageBuffer.Pop()
	if item != nil {
		m.StoragePipeline.Accept(item.(queueing.PipelineItem))
		return true
	}

	item = m.TopDownPreStorageBuffer.Pop()
	if item != nil {
		m.StoragePipeline.Accept(item.(queueing.PipelineItem))
		return true
	}

	return false
}
