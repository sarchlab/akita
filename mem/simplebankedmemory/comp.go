package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type bank struct {
	pending         sim.Buffer
	pipeline        pipelining.Pipeline
	postPipelineBuf sim.Buffer
}

type bankPipelineItem struct {
	req       mem.AccessReq
	committed bool
	readData  []byte
}

func (i *bankPipelineItem) TaskID() string {
	return i.req.Meta().ID
}

// Comp models a banked memory with configurable banking and pipeline behavior.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort sim.Port

	Storage          *mem.Storage
	AddressConverter mem.AddressConverter

	banks        []bank
	bankSelector BankSelector
}

// Tick updates the component state cycle by cycle.
func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

func (m *middleware) Tick() (madeProgress bool) {
	madeProgress = m.acceptIncoming() || madeProgress
	madeProgress = m.finalizeBanks() || madeProgress
	madeProgress = m.tickPipelines() || madeProgress
	madeProgress = m.feedPipelines() || madeProgress

	return madeProgress
}

func (m *middleware) acceptIncoming() bool {
	madeProgress := false

	for {
		msg := m.topPort.PeekIncoming()
		if msg == nil {
			break
		}

		req, ok := msg.(mem.AccessReq)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msg)
		}

		if len(m.banks) == 0 {
			log.Panic("simplebankedmemory: no banks configured")
		}

		bankID := m.bankSelector(req, len(m.banks))
		if bankID < 0 || bankID >= len(m.banks) {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		b := &m.banks[bankID]
		if !b.pending.CanPush() {
			break
		}

		m.topPort.RetrieveIncoming()
		tracing.TraceReqReceive(req, m.Comp)

		item := &bankPipelineItem{req: req}
		b.pending.Push(item)
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

	switch req := item.req.(type) {
	case *mem.ReadReq:
		return m.finalizeRead(b, item, req)
	case *mem.WriteReq:
		return m.finalizeWrite(b, item, req)
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", req)
	}

	return false
}

func (m *middleware) finalizeRead(
	b *bank,
	item *bankPipelineItem,
	req *mem.ReadReq,
) bool {
	if !item.committed {
		addr := req.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		data, err := m.Storage.Read(addr, req.AccessByteSize)
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
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(item.readData).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(req, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) finalizeWrite(
	b *bank,
	item *bankPipelineItem,
	req *mem.WriteReq,
) bool {
	if !item.committed {
		addr := req.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		if req.DirtyMask == nil {
			if err := m.Storage.Write(addr, req.Data); err != nil {
				log.Panic(err)
			}
		} else {
			data, err := m.Storage.Read(addr, uint64(len(req.Data)))
			if err != nil {
				log.Panic(err)
			}

			for i := range req.Data {
				if req.DirtyMask[i] {
					data[i] = req.Data[i]
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
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(req, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) tickPipelines() bool {
	madeProgress := false

	for i := range m.banks {
		madeProgress = m.banks[i].pipeline.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) feedPipelines() bool {
	madeProgress := false

	for i := range m.banks {
		b := &m.banks[i]

		for {
			if !b.pipeline.CanAccept() {
				break
			}

			itemIfc := b.pending.Peek()
			if itemIfc == nil {
				break
			}

			item := itemIfc.(*bankPipelineItem)
			b.pipeline.Accept(item)
			b.pending.Pop()
			madeProgress = true
		}
	}

	return madeProgress
}
