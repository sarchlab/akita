package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type respondStage struct {
	cache *pipelineMW
}

func (s *respondStage) Tick() bool {
	next := &s.cache.comp.State
	if len(next.Transactions) == 0 {
		return false
	}

	for i := 0; i < len(next.Transactions); i++ {
		trans := &next.Transactions[i]
		if !trans.Done || trans.Removed {
			continue
		}

		if trans.HasRead {
			return s.respondReadTrans(trans)
		}

		return s.respondWriteTrans(trans)
	}

	return false
}

func (s *respondStage) respondReadTrans(trans *transactionState) bool {
	dr := messaging.Msg{Payload: memprotocol.DataReadyRsp{
		Data: trans.Data},
		ID:    s.cache.comp.NewID(),
		Src:   s.cache.topPort().AsRemote(),
		Dst:   trans.ReadMeta.Src,
		RspTo: trans.ReadMeta.ID,

		TrafficBytes: len(trans.Data) + 4,
	}

	if !s.cache.topPort().CanSend() {
		return false
	}

	s.cache.topPort().Send(dr)

	trans.Removed = true

	// Trace the original read request.
	s.cache.comp.TaskTracker().EndReqIn(trans.RecvTaskID)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transactionState) bool {
	done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           s.cache.comp.NewID(),
		Src:          s.cache.topPort().AsRemote(),
		Dst:          trans.WriteMeta.Src,
		RspTo:        trans.WriteMeta.ID,
		TrafficBytes: 4,
	}

	if !s.cache.topPort().CanSend() {
		return false
	}

	s.cache.topPort().Send(done)

	trans.Removed = true

	// Trace the original write request.
	s.cache.comp.TaskTracker().EndReqIn(trans.RecvTaskID)

	return true
}
