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
type Spec struct {
	Freq                           sim.Freq `json:"freq"`
	NumBanks                       int      `json:"num_banks"`
	BankPipelineWidth              int    `json:"bank_pipeline_width"`
	BankPipelineDepth              int    `json:"bank_pipeline_depth"`
	StageLatency                   int    `json:"stage_latency"`
	PostPipelineBufSize            int    `json:"post_pipeline_buf_size"`
	BankSelectorKind               string `json:"bank_selector_kind"`
	BankSelectorLog2InterleaveSize uint64 `json:"bank_selector_log2_interleave_size"`
	AddrConvKind                   string `json:"addr_conv_kind"`
	AddrInterleavingSize           uint64 `json:"addr_interleaving_size"`
	AddrTotalNumOfElements         int    `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex        int    `json:"addr_current_element_index"`
	AddrOffset                     uint64 `json:"addr_offset"`
	StorageRef                     string `json:"storage_ref"`
}

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	IsRead    bool         `json:"is_read"`
	ReadMsg   mem.ReadReq  `json:"read_msg"`
	WriteMsg  mem.WriteReq `json:"write_msg"`
	Committed bool         `json:"committed"`
	ReadData  []byte       `json:"read_data"`
}

// bankState captures one bank pipeline + buffer contents.
type bankState struct {
	Pipeline        queueing.Pipeline[bankPipelineItemState] `json:"pipeline"`
	PostPipelineBuf []bankPipelineItemState                  `json:"post_pipeline_buf"`
}

// State contains mutable runtime data for the simple banked memory.
type State struct {
	Banks []bankState `json:"banks"`
}

// Comp models a banked memory with configurable banking and pipeline behavior.
type Comp struct {
	*modeling.Component[Spec, State]

	storage *mem.Storage
}

// GetStorage returns the underlying storage.
func (c *Comp) GetStorage() *mem.Storage {
	return c.storage
}

// StorageName returns the name used to identify this component's storage.
func (c *Comp) StorageName() string {
	return c.GetSpec().StorageRef
}

// --- Free functions for pipeline / buffer / bank-selection / address conversion ---

func pipelineCanAccept(bank bankState, spec Spec) bool {
	if spec.BankPipelineDepth == 0 {
		return len(bank.PostPipelineBuf) < spec.PostPipelineBufSize
	}

	return bank.Pipeline.CanAccept()
}

func pipelineAccept(
	bank *bankState,
	spec Spec,
	item bankPipelineItemState,
) {
	if spec.BankPipelineDepth == 0 {
		bank.PostPipelineBuf = append(bank.PostPipelineBuf, item)
		return
	}

	bank.Pipeline.Accept(item)
}

func pipelineTick(bank *bankState, spec Spec) bool {
	return bank.Pipeline.TickFunc(func(item bankPipelineItemState) bool {
		if len(bank.PostPipelineBuf) < spec.PostPipelineBufSize {
			bank.PostPipelineBuf = append(bank.PostPipelineBuf, item)
			return true
		}

		return false
	})
}

func bufferPeek(bank bankState) (bankPipelineItemState, bool) {
	if len(bank.PostPipelineBuf) == 0 {
		return bankPipelineItemState{}, false
	}

	return bank.PostPipelineBuf[0], true
}

func bufferPop(bank *bankState) {
	if len(bank.PostPipelineBuf) == 0 {
		return
	}

	bank.PostPipelineBuf = bank.PostPipelineBuf[1:]
}

func selectBank(spec Spec, addr uint64) int {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory: invalid interleave size")
	}

	return int((addr / interleaveSize) % uint64(spec.NumBanks))
}

// --- tickFinalizeMW ---

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
		addr := mem.ConvertAddress(spec.AddrConvKind, spec.AddrOffset, spec.AddrInterleavingSize, spec.AddrTotalNumOfElements, spec.AddrCurrentElementIndex, readReq.Address)

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
		addr := mem.ConvertAddress(spec.AddrConvKind, spec.AddrOffset, spec.AddrInterleavingSize, spec.AddrTotalNumOfElements, spec.AddrCurrentElementIndex, writeReq.Address)

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

// --- dispatchMW ---

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
		addr = mem.ConvertAddress(spec.AddrConvKind, spec.AddrOffset, spec.AddrInterleavingSize, spec.AddrTotalNumOfElements, spec.AddrCurrentElementIndex, addr)

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
