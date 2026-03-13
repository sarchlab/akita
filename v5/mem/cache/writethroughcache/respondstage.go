package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type respondStage struct {
	cache *pipelineMW
}

func (s *respondStage) Tick() bool {
	next := s.cache.comp.GetNextState()
	if next.NumTransactions == 0 {
		return false
	}

	for i := 0; i < next.NumTransactions; i++ {
		trans := &next.Transactions[i]
		if !trans.Done {
			continue
		}

		if trans.HasRead {
			return s.respondReadTrans(trans, i)
		}

		return s.respondWriteTrans(trans, i)
	}

	return false
}

func (s *respondStage) respondReadTrans(trans *transactionState, idx int) bool {
	if !trans.Done {
		return false
	}

	dr := &mem.DataReadyRsp{}
	dr.ID = sim.GetIDGenerator().Generate()
	dr.Src = s.cache.topPort.AsRemote()
	dr.Dst = trans.ReadMeta.Src
	dr.RspTo = trans.ReadMeta.ID
	dr.Data = trans.Data
	dr.TrafficBytes = len(trans.Data) + 4
	dr.TrafficClass = "rsp"

	err := s.cache.topPort.Send(dr)
	if err != nil {
		return false
	}

	s.removeTransaction(idx)

	// Reconstruct read for tracing
	read := &mem.ReadReq{
		MsgMeta:        trans.ReadMeta,
		Address:        trans.ReadAddress,
		AccessByteSize: trans.ReadAccessByteSize,
		PID:            trans.ReadPID,
	}
	tracing.TraceReqComplete(read, s.cache.comp)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transactionState, idx int) bool {
	if !trans.Done {
		return false
	}

	done := &mem.WriteDoneRsp{}
	done.ID = sim.GetIDGenerator().Generate()
	done.Src = s.cache.topPort.AsRemote()
	done.Dst = trans.WriteMeta.Src
	done.RspTo = trans.WriteMeta.ID
	done.TrafficBytes = 4
	done.TrafficClass = "rsp"

	err := s.cache.topPort.Send(done)
	if err != nil {
		return false
	}

	s.removeTransaction(idx)

	// Reconstruct write for tracing
	write := &mem.WriteReq{
		MsgMeta:   trans.WriteMeta,
		Address:   trans.WriteAddress,
		Data:      trans.WriteData,
		DirtyMask: trans.WriteDirtyMask,
		PID:       trans.WritePID,
	}
	tracing.TraceReqComplete(write, s.cache.comp)

	return true
}

// removeTransaction removes a pre-coalesce transaction at the given absolute
// index from State.Transactions. It also updates all PreCoalesceTransIdxs
// in post-coalesce transactions to reflect the shifted indices.
func (s *respondStage) removeTransaction(idx int) {
	next := s.cache.comp.GetNextState()

	// Remove the pre-coalesce transaction
	next.Transactions = append(next.Transactions[:idx],
		next.Transactions[idx+1:]...)
	next.NumTransactions--

	// Update all PreCoalesceTransIdxs in post-coalesce transactions
	for i := next.NumTransactions; i < len(next.Transactions); i++ {
		pct := &next.Transactions[i]
		for j := range pct.PreCoalesceTransIdxs {
			if pct.PreCoalesceTransIdxs[j] > idx {
				pct.PreCoalesceTransIdxs[j]--
			}
		}
	}
}
