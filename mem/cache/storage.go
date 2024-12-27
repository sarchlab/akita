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
	for range m.numReqPerCycle {
		madeProgress = m.processPostPipelineBuffer() || madeProgress
	}

	madeProgress = m.storagePipeline.Tick() || madeProgress

	for range m.numReqPerCycle {
		madeProgress = m.processPrePipelineBuffer() || madeProgress
	}

	return
}

func (m *storageMiddleware) processPostPipelineBuffer() bool {
	panic("not implemented")
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
