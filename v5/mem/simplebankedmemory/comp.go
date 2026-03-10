package simplebankedmemory

import (
	"io"
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct{}

// msgRef is a serializable representation of a *sim.Msg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// bankPipelineItemState is a serializable representation of bankPipelineItem.
type bankPipelineItemState struct {
	Msg       msgRef `json:"msg"`
	Committed bool   `json:"committed"`
	ReadData  []byte `json:"read_data"`
}

// bankPipelineStageState captures one non-nil pipeline slot.
type bankPipelineStageState struct {
	Lane      int                   `json:"lane"`
	Stage     int                   `json:"stage"`
	Item      bankPipelineItemState `json:"item"`
	CycleLeft int                   `json:"cycle_left"`
}

// bankState captures one bank pipeline + buffer contents.
type bankState struct {
	PipelineStages  []bankPipelineStageState `json:"pipeline_stages"`
	PostPipelineBuf []bankPipelineItemState  `json:"post_pipeline_buf"`
}

// State contains mutable runtime data for the simple banked memory.
type State struct {
	Banks []bankState `json:"banks"`
}

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

func msgRefFromMsg(m *sim.Msg) msgRef {
	return msgRef{
		ID:           m.ID,
		Src:          m.Src,
		Dst:          m.Dst,
		RspTo:        m.RspTo,
		TrafficClass: m.TrafficClass,
		TrafficBytes: m.TrafficBytes,
	}
}

func msgFromRef(r msgRef) *sim.Msg {
	return &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:           r.ID,
			Src:          r.Src,
			Dst:          r.Dst,
			TrafficClass: r.TrafficClass,
			TrafficBytes: r.TrafficBytes,
		},
		RspTo: r.RspTo,
	}
}

func bankPipelineItemStateFromItem(item *bankPipelineItem) bankPipelineItemState {
	return bankPipelineItemState{
		Msg:       msgRefFromMsg(item.msg),
		Committed: item.committed,
		ReadData:  item.readData,
	}
}

func bankPipelineItemFromState(s bankPipelineItemState) *bankPipelineItem {
	return &bankPipelineItem{
		msg:       msgFromRef(s.Msg),
		committed: s.Committed,
		readData:  s.ReadData,
	}
}

// snapshotState converts runtime mutable data into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{
		Banks: make([]bankState, len(c.banks)),
	}

	for i, b := range c.banks {
		pipeSnaps := queueing.SnapshotPipeline(b.pipeline)
		stages := make([]bankPipelineStageState, len(pipeSnaps))

		for j, snap := range pipeSnaps {
			item := snap.Elem.(*bankPipelineItem)
			stages[j] = bankPipelineStageState{
				Lane:      snap.Lane,
				Stage:     snap.Stage,
				Item:      bankPipelineItemStateFromItem(item),
				CycleLeft: snap.CycleLeft,
			}
		}

		bufElems := queueing.SnapshotBuffer(b.postPipelineBuf)
		bufItems := make([]bankPipelineItemState, len(bufElems))

		for j, elem := range bufElems {
			item := elem.(*bankPipelineItem)
			bufItems[j] = bankPipelineItemStateFromItem(item)
		}

		s.Banks[i] = bankState{
			PipelineStages:  stages,
			PostPipelineBuf: bufItems,
		}
	}

	return s
}

// restoreFromState restores runtime mutable data from a serializable State.
func (c *Comp) restoreFromState(s State) {
	for i, bs := range s.Banks {
		b := &c.banks[i]

		pipeSnaps := make([]queueing.PipelineStageSnapshot, len(bs.PipelineStages))
		for j, stage := range bs.PipelineStages {
			pipeSnaps[j] = queueing.PipelineStageSnapshot{
				Lane:      stage.Lane,
				Stage:     stage.Stage,
				Elem:      bankPipelineItemFromState(stage.Item),
				CycleLeft: stage.CycleLeft,
			}
		}

		queueing.RestorePipeline(b.pipeline, pipeSnaps)

		bufElems := make([]interface{}, len(bs.PostPipelineBuf))
		for j, item := range bs.PostPipelineBuf {
			bufElems[j] = bankPipelineItemFromState(item)
		}

		queueing.RestoreBuffer(b.postPipelineBuf, bufElems)
	}
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := c.snapshotState()
	c.Component.SetState(state)
	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)
	c.restoreFromState(state)
}

// SaveState marshals the component's spec and state as JSON, ensuring the
// runtime fields are synced into State first.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState reads JSON from r and restores both the base state and the
// runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}
	c.SetState(c.Component.GetState())
	return nil
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
