package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct {
	NumBanks                       int    `json:"num_banks"`
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

	for lane := 0; lane < spec.BankPipelineWidth; lane++ {
		if !pipelineSlotOccupied(bank, lane, 0) {
			return true
		}
	}

	return false
}

func pipelineSlotOccupied(bank bankState, lane, stage int) bool {
	for _, s := range bank.PipelineStages {
		if s.Lane == lane && s.Stage == stage {
			return true
		}
	}

	return false
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

	for lane := 0; lane < spec.BankPipelineWidth; lane++ {
		if !pipelineSlotOccupied(*bank, lane, 0) {
			bank.PipelineStages = append(bank.PipelineStages,
				bankPipelineStageState{
					Lane:      lane,
					Stage:     0,
					Item:      item,
					CycleLeft: spec.StageLatency - 1,
				})
			return
		}
	}

	panic("pipeline is full, call pipelineCanAccept first")
}

// pipelineTick advances items through the pipeline stages.
// Items enter at stage 0 and advance towards stage (depth-1).
// When an item finishes at the last stage, it moves to PostPipelineBuf.
func pipelineTick(bank *bankState, spec Spec) bool {
	madeProgress := false
	remaining := make([]bankPipelineStageState, 0, len(bank.PipelineStages))

	// Process from the last stage backwards for proper advancement.
	// We collect items to keep and advance in multiple passes.

	// Sort stages by stage number descending so we process the last stages first.
	// We'll iterate from highest stage to lowest.
	lastStage := spec.BankPipelineDepth - 1

	// First pass: mark which stages are occupied after processing
	type action int
	const (
		actionKeep action = iota
		actionAdvanced
		actionMoveToBuffer
	)

	actions := make([]action, len(bank.PipelineStages))
	newStages := make([]bankPipelineStageState, len(bank.PipelineStages))
	copy(newStages, bank.PipelineStages)

	// Process from highest stage to stage 0
	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != actionKeep {
				continue
			}

			s := &newStages[i]
			if s.Stage != stageNum {
				continue
			}

			if s.CycleLeft > 0 {
				s.CycleLeft--
				madeProgress = true
				continue
			}

			// CycleLeft == 0, try to advance
			if stageNum == lastStage {
				// Try to move to post-pipeline buffer
				if len(bank.PostPipelineBuf) < spec.PostPipelineBufSize {
					bank.PostPipelineBuf = append(
						bank.PostPipelineBuf, s.Item)
					actions[i] = actionMoveToBuffer
					madeProgress = true
				}
			} else {
				// Try to move to next stage
				nextStageNum := stageNum + 1
				nextOccupied := false

				for j := range newStages {
					if actions[j] != actionKeep {
						continue
					}

					if newStages[j].Lane == s.Lane &&
						newStages[j].Stage == nextStageNum {
						nextOccupied = true
						break
					}
				}

				if !nextOccupied {
					s.Stage = nextStageNum
					s.CycleLeft = spec.StageLatency - 1
					actions[i] = actionAdvanced
					madeProgress = true
				}
			}
		}
	}

	for i, a := range actions {
		if a == actionMoveToBuffer {
			continue
		}

		s := newStages[i]
		if a == actionAdvanced {
			// Already updated stage/cycleLeft
		}

		remaining = append(remaining, s)
	}

	bank.PipelineStages = remaining

	return madeProgress
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

func convertAddress(spec Spec, addr uint64) uint64 {
	if spec.AddrConvKind == "" {
		return addr
	}

	if addr < spec.AddrOffset {
		log.Panic("address is smaller than offset")
	}

	a := addr - spec.AddrOffset
	roundSize := spec.AddrInterleavingSize * uint64(spec.AddrTotalNumOfElements)
	belongsTo := int(a % roundSize / spec.AddrInterleavingSize)

	if belongsTo != spec.AddrCurrentElementIndex {
		log.Panicf("address 0x%x does not belong to current element %d",
			addr, spec.AddrCurrentElementIndex)
	}

	return a/roundSize*spec.AddrInterleavingSize +
		addr%spec.AddrInterleavingSize
}

// --- Middleware ---

type middleware struct {
	comp    *modeling.Component[Spec, State]
	storage *mem.Storage
}

func (m *middleware) Name() string {
	return m.comp.Name()
}

func (m *middleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *middleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *middleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *middleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *middleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *middleware) Tick() (madeProgress bool) {
	madeProgress = m.finalizeBanks() || madeProgress
	madeProgress = m.tickPipelines() || madeProgress
	madeProgress = m.dispatchFromTopPort() || madeProgress

	return madeProgress
}

func (m *middleware) dispatchFromTopPort() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	nextState := m.comp.GetNextState()

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
		addr = convertAddress(spec, addr)

		bankID := selectBank(spec, addr)
		if bankID < 0 || bankID >= spec.NumBanks {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		b := &nextState.Banks[bankID]
		if !pipelineCanAccept(*b, spec) {
			break
		}

		m.topPort().RetrieveIncoming()
		tracing.TraceReqReceive(msg, m)

		item := m.msgToItem(msg)
		pipelineAccept(b, spec, item)
		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) msgToItem(msg sim.Msg) bankPipelineItemState {
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

func (m *middleware) finalizeBanks() bool {
	madeProgress := false
	nextState := m.comp.GetNextState()

	for i := range nextState.Banks {
		for {
			progress := m.finalizeSingle(&nextState.Banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) finalizeSingle(b *bankState) bool {
	item, ok := bufferPeek(*b)
	if !ok {
		return false
	}

	if item.IsRead {
		return m.finalizeRead(b, &item)
	}

	return m.finalizeWrite(b, &item)
}

func (m *middleware) finalizeRead(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	spec := m.comp.GetSpec()
	readReq := &item.ReadMsg

	if !item.Committed {
		addr := convertAddress(spec, readReq.Address)

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

	tracing.TraceReqComplete(&item.ReadMsg, m)

	bufferPop(b)

	return true
}

func (m *middleware) finalizeWrite(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	spec := m.comp.GetSpec()
	writeReq := &item.WriteMsg

	if !item.Committed {
		addr := convertAddress(spec, writeReq.Address)

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

	tracing.TraceReqComplete(&item.WriteMsg, m)

	bufferPop(b)

	return true
}

func (m *middleware) tickPipelines() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	nextState := m.comp.GetNextState()

	for i := range nextState.Banks {
		madeProgress = pipelineTick(&nextState.Banks[i], spec) || madeProgress
	}

	return madeProgress
}
