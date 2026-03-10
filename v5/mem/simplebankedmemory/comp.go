package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct{}

// State contains mutable runtime data for the simple banked memory.
type State struct{}

type bank struct {
	pipeline        queueing.Pipeline
	postPipelineBuf queueing.Buffer
}

type bankPipelineItem struct {
	msg       *sim.Msg
	committed bool
	readData  []byte
}

func (i *bankPipelineItem) TaskID() string {
	return i.msg.ID + "_pl"
}

// Comp models a banked memory with configurable banking and pipeline behavior.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort sim.Port

	Storage          *mem.Storage
	AddressConverter mem.AddressConverter

	banks        []bank
	bankSelector bankSelector
}

type middleware struct {
	*Comp
}

func (m *middleware) Tick() (madeProgress bool) {
	madeProgress = m.finalizeBanks() || madeProgress
	madeProgress = m.tickPipelines() || madeProgress
	madeProgress = m.dispatchFromTopPort() || madeProgress

	return madeProgress
}

func (m *middleware) dispatchFromTopPort() bool {
	madeProgress := false

	for {
		msg := m.topPort.PeekIncoming()
		if msg == nil {
			break
		}

		payload, ok := msg.Payload.(mem.AccessReqPayload)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msg.Payload)
		}

		if len(m.banks) == 0 {
			log.Panic("simplebankedmemory: no banks configured")
		}

		addr := payload.GetAddress()
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		bankID := m.bankSelector.Select(addr, len(m.banks))
		if bankID < 0 || bankID >= len(m.banks) {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		b := &m.banks[bankID]
		if !b.pipeline.CanAccept() {
			break
		}

		m.topPort.RetrieveIncoming()
		tracing.TraceReqReceive(msg, m.Comp)

		item := &bankPipelineItem{msg: msg}
		b.pipeline.Accept(item)
		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) finalizeBanks() bool {
	madeProgress := false

	for i := range m.banks {
		for {
			progress := m.finalizeSingle(&m.banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) finalizeSingle(b *bank) bool {
	itemIfc := b.postPipelineBuf.Peek()
	if itemIfc == nil {
		return false
	}

	item := itemIfc.(*bankPipelineItem)

	switch item.msg.Payload.(type) {
	case *mem.ReadReqPayload:
		return m.finalizeRead(b, item)
	case *mem.WriteReqPayload:
		return m.finalizeWrite(b, item)
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T",
			item.msg.Payload)
	}

	return false
}

func (m *middleware) finalizeRead(
	b *bank,
	item *bankPipelineItem,
) bool {
	msg := item.msg
	readPayload := sim.MsgPayload[mem.ReadReqPayload](msg)

	if !item.committed {
		addr := readPayload.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		data, err := m.Storage.Read(addr, readPayload.AccessByteSize)
		if err != nil {
			log.Panic(err)
		}

		item.readData = data
		item.committed = true
	}

	if !m.topPort.CanSend() {
		return false
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(msg.Src).
		WithRspTo(msg.ID).
		WithData(item.readData).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(msg, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) finalizeWrite(
	b *bank,
	item *bankPipelineItem,
) bool {
	msg := item.msg
	writePayload := sim.MsgPayload[mem.WriteReqPayload](msg)

	if !item.committed {
		addr := writePayload.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		if writePayload.DirtyMask == nil {
			if err := m.Storage.Write(addr, writePayload.Data); err != nil {
				log.Panic(err)
			}
		} else {
			data, err := m.Storage.Read(addr, uint64(len(writePayload.Data)))
			if err != nil {
				log.Panic(err)
			}

			for i := range writePayload.Data {
				if writePayload.DirtyMask[i] {
					data[i] = writePayload.Data[i]
				}
			}

			if err := m.Storage.Write(addr, data); err != nil {
				log.Panic(err)
			}
		}

		item.committed = true
	}

	if !m.topPort.CanSend() {
		return false
	}

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(msg.Src).
		WithRspTo(msg.ID).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(msg, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) tickPipelines() bool {
	madeProgress := false

	for i := range m.banks {
		p := m.banks[i].pipeline
		madeProgress = p.Tick() || madeProgress
	}

	return madeProgress
}
