package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type dispatchMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *dispatchMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *dispatchMW) Tick() bool {
	return m.dispatchFromTopPort()
}

func (m *dispatchMW) dispatchFromTopPort() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for {
		msgI := m.topPort().PeekIncoming()
		if msgI == nil {
			break
		}

		msg, ok := msgI.(mem.AccessReq)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msgI)
		}

		if spec.NumBanks == 0 {
			log.Panic("simplebankedmemory: no banks configured")
		}

		addr := msg.GetAddress()
		addr = mem.ConvertAddress(
			spec.AddrConvKind, spec.AddrOffset,
			spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
			spec.AddrCurrentElementIndex, addr,
		)

		bankID := selectBank(spec, addr)
		if bankID < 0 || bankID >= spec.NumBanks {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		if !pipelineCanAccept(next.Banks[bankID], spec) {
			break
		}

		m.topPort().RetrieveIncoming()
		tracing.TraceReqReceive(msg, m.comp)

		item := m.msgToItem(msg)
		pipelineAccept(&next.Banks[bankID], spec, item)
		madeProgress = true
	}

	return madeProgress
}

func (m *dispatchMW) msgToItem(msg sim.Msg) bankPipelineItemState {
	switch r := msg.(type) {
	case *mem.ReadReq:
		return bankPipelineItemState{
			IsRead:  true,
			ReadMsg: *r,
		}
	case *mem.WriteReq:
		return bankPipelineItemState{
			IsRead:   false,
			WriteMsg: *r,
		}
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", msg)
		return bankPipelineItemState{}
	}
}
