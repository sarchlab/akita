package writethroughcache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type intake struct {
	cache *pipelineMW
}

func (s *intake) Tick() bool {
	msg := s.cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	next := &s.cache.comp.State
	if s.countActive(next) >= s.cache.comp.Spec().MaxNumConcurrentTrans {
		return false
	}

	dirBuf := &next.DirBuf
	if !dirBuf.CanPush() {
		return false
	}

	transIdx := s.createTransaction(msg)

	dirBuf.PushTyped(transIdx)

	s.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(s.cache.comp, msg)

	return true
}

func (s *intake) countActive(state *State) int {
	count := 0
	for i := range state.Transactions {
		if !state.Transactions[i].Removed {
			count++
		}
	}
	return count
}

func (s *intake) createTransaction(msg messaging.Msg) int {
	next := &s.cache.comp.State

	var t transactionState
	switch m := msg.(type) {
	case mem.ReadReq:
		t = transactionState{
			ID:                 timing.GetIDGenerator().Generate(),
			HasRead:            true,
			ReadMeta:           m.MsgMeta,
			ReadAddress:        m.Address,
			ReadAccessByteSize: m.AccessByteSize,
			ReadPID:            m.PID,
		}

		tracing.StartTask(s.cache.comp, tracing.TaskStart{
			ID:       t.ID,
			ParentID: tracing.MsgIDAtReceiver(m, s.cache.comp),
			Kind:     "cache_transaction",
			What:     "read",
			Location: s.cache.comp.Name() + ".Local",
		})
	case mem.WriteReq:
		t = transactionState{
			ID:             timing.GetIDGenerator().Generate(),
			HasWrite:       true,
			WriteMeta:      m.MsgMeta,
			WriteAddress:   m.Address,
			WriteData:      m.Data,
			WriteDirtyMask: m.DirtyMask,
			WritePID:       m.PID,
		}

		if t.WriteDirtyMask == nil {
			t.WriteDirtyMask = make([]bool, len(m.Data))
			for i := range t.WriteDirtyMask {
				t.WriteDirtyMask[i] = true
			}
		}

		tracing.StartTask(s.cache.comp, tracing.TaskStart{
			ID:       t.ID,
			ParentID: tracing.MsgIDAtReceiver(m, s.cache.comp),
			Kind:     "cache_transaction",
			What:     "write",
			Location: s.cache.comp.Name() + ".Local",
		})
	default:
		log.Panicf("cannot process request of type %s\n",
			reflect.TypeOf(msg))
		return -1
	}

	next.Transactions = append(next.Transactions, t)
	return len(next.Transactions) - 1
}
