package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct {
	Freq                timing.Freq `json:"freq"`
	NumBanks            int         `json:"num_banks"`
	BankPipelineWidth   int         `json:"bank_pipeline_width"`
	BankPipelineDepth   int         `json:"bank_pipeline_depth"`
	StageLatency        int         `json:"stage_latency"`
	PostPipelineBufSize int         `json:"post_pipeline_buf_size"`

	// Capacity is the size in bytes of the memory. The component does not
	// read it: the system builder sizes the Resources.Storage it supplies
	// with it.
	Capacity                       uint64 `json:"capacity"`
	BankSelectorKind               string `json:"bank_selector_kind"`
	BankSelectorLog2InterleaveSize uint64 `json:"bank_selector_log2_interleave_size"`

	// Bank-selection address conversion. Bank selection runs on this
	// conversion of the request address; storage is always global, so this
	// conversion affects bank selection only. With an empty BankAddrConvKind
	// (the default) bank selection runs on the global address directly — fine
	// for a standalone memory, or when the bank stride is coarser than any
	// upstream inter-controller interleave. When this memory is one of several
	// controllers interleaved at a finer granularity (e.g. 64 B banks behind a
	// 128 B controller stride), the global address is strided and a contiguous
	// bank selector cannot stripe finely on it. Set these fields (kind
	// "interleaving" with this controller's element index) to strip the
	// inter-controller interleaving so bank selection sees a contiguous
	// controller-local address and stripes across all banks.
	BankAddrConvKind            string `json:"bank_addr_conv_kind"`
	BankAddrInterleavingSize    uint64 `json:"bank_addr_interleaving_size"`
	BankAddrTotalNumOfElements  int    `json:"bank_addr_total_num_of_elements"`
	BankAddrCurrentElementIndex int    `json:"bank_addr_current_element_index"`
	BankAddrOffset              uint64 `json:"bank_addr_offset"`
}

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	IsRead    bool                 `json:"is_read"`
	ReadMsg   memprotocol.ReadReq  `json:"read_msg"`
	WriteMsg  memprotocol.WriteReq `json:"write_msg"`
	Committed bool                 `json:"committed"`
	ReadData  []byte               `json:"read_data"`
	// PipelineTaskID is the ID of the PipelineTaskKind subtask opened on the
	// req_in at dispatch (pipeline entry) and closed at finalize (pipeline
	// exit). It travels with the item through the bank pipeline so the exit
	// side can close the same task it opened. Zero when tracing is disabled.
	PipelineTaskID uint64 `json:"pipeline_task_id"`
}

// bankState captures one bank pipeline + buffer contents.
type bankState struct {
	Pipeline        queueing.Pipeline[bankPipelineItemState] `json:"pipeline"`
	PostPipelineBuf queueing.Buffer[bankPipelineItemState]   `json:"post_pipeline_buf"`
}

// state contains mutable runtime data for the simple banked memory.
type state struct {
	ControlState  memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID  uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort     `json:"current_cmd_src"`
	Banks         []bankState              `json:"banks"`
}

// Resources holds the shared resources referenced by the memory.
type Resources struct {
	// Storage holds the memory's data. It is required; the system builder
	// creates it (usually sized by Spec.Capacity) and may share it with other
	// components.
	Storage *mem.Storage
}

// Ports holds the memory's ports.
type Ports struct {
	// Top receives read and write requests and returns their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Control receives enable, pause, drain, and reset commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the memory's behavior, run in field order every cycle.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// TickFinalize commits requests leaving the bank pipelines, responds to
	// them, and advances the pipelines.
	TickFinalize *tickFinalizeMW

	// Dispatch moves incoming requests into their banks' pipelines.
	Dispatch *dispatchMW
}

// Comp models a banked memory with configurable banking and pipeline behavior.
// It is a ticking component specialized to this package's five structs.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]

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
		bank.PostPipelineBuf.Push(item)
		return
	}

	bank.Pipeline.Accept(item)
}

func pipelineTick(bank *bankState) bool {
	return bank.Pipeline.Tick(&bank.PostPipelineBuf)
}

func bufferPeek(bank bankState) (bankPipelineItemState, bool) {
	return bank.PostPipelineBuf.Peek()
}

func bufferPop(bank *bankState) {
	if bank.PostPipelineBuf.Size() == 0 {
		return
	}

	bank.PostPipelineBuf.Pop()
}

// selectBank chooses the bank for a (bank-selection) address. Callers pass the
// result of bankSelectionAddress, not the raw request address.
func selectBank(spec Spec, addr uint64) int {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory: invalid interleave size")
	}

	return int((addr / interleaveSize) % uint64(spec.NumBanks))
}

// bankSelectionAddress converts a request address to the address fed to
// selectBank. Storage is global and is never converted; this conversion
// affects bank selection only. With an empty BankAddrConvKind it is the
// identity, so bank selection runs on the global address. Set the BankAddr*
// fields to strip an upstream inter-controller interleaving so banks stripe on
// the contiguous controller-local address. See the BankAddrConv* Spec fields.
func bankSelectionAddress(spec Spec, addr uint64) uint64 {
	return mem.ConvertAddress(
		spec.BankAddrConvKind, spec.BankAddrOffset,
		spec.BankAddrInterleavingSize, spec.BankAddrTotalNumOfElements,
		spec.BankAddrCurrentElementIndex, addr,
	)
}
