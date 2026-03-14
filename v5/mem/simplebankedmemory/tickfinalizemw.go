package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type tickFinalizeMW struct {
	comp    *modeling.Component[Spec, State]
	storage *mem.Storage
}

func (m *tickFinalizeMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *tickFinalizeMW) Tick() bool {
	madeProgress := m.finalizeBanks()
	madeProgress = m.tickPipelines() || madeProgress
	return madeProgress
}

func (m *tickFinalizeMW) finalizeBanks() bool {
	madeProgress := false
	cur := m.comp.GetState()
	next := m.comp.GetNextState()

	for i := range cur.Banks {
		for {
			progress := m.finalizeSingle(&next.Banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *tickFinalizeMW) finalizeSingle(b *bankState) bool {
	item, ok := bufferPeek(*b)
	if !ok {
		return false
	}

	if item.IsRead {
		return m.finalizeRead(b, &item)
	}

	return m.finalizeWrite(b, &item)
}

func (m *tickFinalizeMW) finalizeRead(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	spec := m.comp.GetSpec()
	readReq := &item.ReadMsg

	if !item.Committed {
		addr := mem.ConvertAddress(
			spec.AddrConvKind, spec.AddrOffset,
			spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
			spec.AddrCurrentElementIndex, readReq.Address,
		)

		data, err := m.storage.Read(addr, readReq.AccessByteSize)
		if err != nil {
			log.Panic(err)
		}

		item.ReadData = data
		item.Committed = true

		// Update the buffer head with the committed state
		b.PostPipelineBuf[0] = *item
	}

	if !m.topPort().CanSend() {
		return false
	}

	rsp := &mem.DataReadyRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = readReq.Src
	rsp.RspTo = readReq.ID
	rsp.Data = item.ReadData
	rsp.TrafficBytes = len(item.ReadData) + 4
	rsp.TrafficClass = "mem.DataReadyRsp"

	if err := m.topPort().Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(&item.ReadMsg, m.comp)

	bufferPop(b)

	return true
}

func (m *tickFinalizeMW) finalizeWrite(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	spec := m.comp.GetSpec()
	writeReq := &item.WriteMsg

	if !item.Committed {
		addr := mem.ConvertAddress(
			spec.AddrConvKind, spec.AddrOffset,
			spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
			spec.AddrCurrentElementIndex, writeReq.Address,
		)

		if writeReq.DirtyMask == nil {
			if err := m.storage.Write(addr, writeReq.Data); err != nil {
				log.Panic(err)
			}
		} else {
			data, err := m.storage.Read(addr, uint64(len(writeReq.Data)))
			if err != nil {
				log.Panic(err)
			}

			for i := range writeReq.Data {
				if writeReq.DirtyMask[i] {
					data[i] = writeReq.Data[i]
				}
			}

			if err := m.storage.Write(addr, data); err != nil {
				log.Panic(err)
			}
		}

		item.Committed = true
		b.PostPipelineBuf[0] = *item
	}

	if !m.topPort().CanSend() {
		return false
	}

	rsp := &mem.WriteDoneRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = writeReq.Src
	rsp.RspTo = writeReq.ID
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.WriteDoneRsp"

	if err := m.topPort().Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(&item.WriteMsg, m.comp)

	bufferPop(b)

	return true
}

func (m *tickFinalizeMW) tickPipelines() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()
	next := m.comp.GetNextState()

	for i := range cur.Banks {
		madeProgress = pipelineTick(&next.Banks[i], spec) || madeProgress
	}

	return madeProgress
}
