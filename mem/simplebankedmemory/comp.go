package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct {
	Freq                           timing.Freq `json:"freq"`
	NumBanks                       int         `json:"num_banks"`
	BankPipelineWidth              int         `json:"bank_pipeline_width"`
	BankPipelineDepth              int         `json:"bank_pipeline_depth"`
	StageLatency                   int         `json:"stage_latency"`
	PostPipelineBufSize            int         `json:"post_pipeline_buf_size"`
	Capacity                       uint64      `json:"capacity"`
	BankSelectorKind               string      `json:"bank_selector_kind"`
	BankSelectorLog2InterleaveSize uint64      `json:"bank_selector_log2_interleave_size"`
	AddrConvKind                   string      `json:"addr_conv_kind"`
	AddrInterleavingSize           uint64      `json:"addr_interleaving_size"`
	AddrTotalNumOfElements         int         `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex        int         `json:"addr_current_element_index"`
	AddrOffset                     uint64      `json:"addr_offset"`
	StorageRef                     string      `json:"storage_ref"`
}

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	IsRead    bool                 `json:"is_read"`
	ReadMsg   memprotocol.ReadReq  `json:"read_msg"`
	WriteMsg  memprotocol.WriteReq `json:"write_msg"`
	Committed bool                 `json:"committed"`
	ReadData  []byte               `json:"read_data"`
}

// bankState captures one bank pipeline + buffer contents.
type bankState struct {
	Pipeline        queueing.Pipeline[bankPipelineItemState] `json:"pipeline"`
	PostPipelineBuf queueing.Buffer[bankPipelineItemState]   `json:"post_pipeline_buf"`
}

// State contains mutable runtime data for the simple banked memory.
type State struct {
	ControlState  control.State        `json:"control_state"`
	CurrentCmdID  uint64               `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort `json:"current_cmd_src"`
	Banks         []bankState          `json:"banks"`
}

// Resources holds the shared resources referenced by the memory.
type Resources struct {
	Storage *mem.Storage
}

// Comp models a banked memory with configurable banking and pipeline behavior.
// It is a modeling.Component specialized to this package's Spec, State, and
// Resources.
type Comp = modeling.Component[Spec, State, Resources]

// --- Free functions for pipeline / buffer / bank-selection / address conversion ---

func pipelineCanAccept(bank bankState, spec Spec) bool {
	if spec.BankPipelineDepth == 0 {
		return bank.PostPipelineBuf.CanPush()
	}

	return bank.Pipeline.CanAccept()
}

func pipelineAccept(
	bank *bankState,
	spec Spec,
	item bankPipelineItemState,
) {
	if spec.BankPipelineDepth == 0 {
		bank.PostPipelineBuf.PushTyped(item)
		return
	}

	bank.Pipeline.Accept(item)
}

func pipelineTick(bank *bankState) bool {
	return bank.Pipeline.Tick(&bank.PostPipelineBuf)
}

func bufferPeek(bank bankState) (bankPipelineItemState, bool) {
	if bank.PostPipelineBuf.Size() == 0 {
		return bankPipelineItemState{}, false
	}

	return bank.PostPipelineBuf.Peek(), true
}

func bufferPop(bank *bankState) {
	if bank.PostPipelineBuf.Size() == 0 {
		return
	}

	bank.PostPipelineBuf.Pop()
}

func selectBank(spec Spec, addr uint64) int {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory: invalid interleave size")
	}

	return int((addr / interleaveSize) % uint64(spec.NumBanks))
}
