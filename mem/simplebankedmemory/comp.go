package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the simple banked memory.
type Spec struct {
	Freq              timing.Freq `json:"freq"`
	NumBanks          int         `json:"num_banks"`
	BankPipelineWidth int         `json:"bank_pipeline_width"`
	BankPipelineDepth int         `json:"bank_pipeline_depth"`
	StageLatency      int         `json:"stage_latency"`

	// Deprecated: PostPipelineBufSize is ignored for every value, including
	// zero and negative ones, and remains for source and JSON compatibility.
	// Completed requests now wait in the final stage of their bank pipeline,
	// so a blocked response backpressures that lane.
	PostPipelineBufSize int `json:"post_pipeline_buf_size"`

	BankSelectorLog2InterleaveSize uint64 `json:"bank_selector_log2_interleave_size"`

	// BankAddressConversion affects bank selection only; storage uses global addresses.
	// The zero value selects banks directly from the global address.
	BankAddressConversion BankAddressConversion `json:"bank_address_conversion"`
}

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	RecvTaskID uint64 `json:"recv_task_id"`

	IsRead    bool          `json:"is_read"`
	ReadMsg   messaging.Msg `json:"read_msg"`
	WriteMsg  messaging.Msg `json:"write_msg"`
	Committed bool          `json:"committed"`
	ReadData  []byte        `json:"read_data"`
	// PipelineTaskID is the ID of the PipelineTaskKind subtask opened on the
	// req_in at dispatch (pipeline entry) and closed at finalize (pipeline
	// exit). It travels with the item through the bank pipeline so the exit
	// side can close the same task it opened. Zero when tracing is disabled.
	PipelineTaskID uint64 `json:"pipeline_task_id"`
}

// bankState captures one bank pipeline. A completed item stays in the final
// stage, with its committed state, until its response is sent.
type bankState struct {
	Pipeline queueing.FastPipeline[bankPipelineItemState] `json:"pipeline"`
}

// state contains mutable runtime data for the simple banked memory.
type state struct {
	ControlState  controlState         `json:"control_state"`
	CurrentCmdID  uint64               `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort `json:"current_cmd_src"`
	Banks         []bankState          `json:"banks"`
}

// Resources holds the shared resources referenced by the memory.
type Resources struct {
	// Storage holds the memory's data. It is required; the system builder
	// sizes it and may share it with other
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

// middlewares holds the memory's behavior, run in field order on every tick.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// TickFinalize commits the requests that are ready and responds to them.
	TickFinalize *tickFinalizeMW

	// Dispatch moves incoming requests into their banks' pipelines.
	Dispatch *dispatchMW

	// Wake schedules the next cycle at which a bank can do work.
	Wake *wakeMW
}

// Comp models a banked memory with configurable banking and pipeline behavior.
// It is a ticking component specialized to this package's five structs.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]

// --- Free functions for pipeline shape / bank-selection / address conversion ---

// bankPipelineShape returns the width and stage count of a bank pipeline. A
// request accepted in cycle c responds in cycle c + numStages when unblocked,
// and the final stage is the only place a completed request waits. Without
// pipeline latency, the pipeline is one stage with at least one lane.
func bankPipelineShape(spec Spec) (width, numStages int) {
	latency := spec.BankPipelineDepth * spec.StageLatency
	if latency <= 0 {
		return max(spec.BankPipelineWidth, 1), 1
	}

	return spec.BankPipelineWidth, latency
}

// requestBank returns the bank that serves a read or write request.
func requestBank(spec Spec, msg messaging.Msg) int {
	var address uint64
	switch req := msg.Payload.(type) {
	case memprotocol.ReadReq:
		address = req.Address
	case memprotocol.WriteReq:
		address = req.Address
	default:
		log.Panicf("simplebankedmemory: unsupported message type %T", msg.Payload)
	}

	if spec.NumBanks == 0 {
		log.Panic("simplebankedmemory: no banks configured")
	}

	bankID := selectBank(spec, bankSelectionAddress(spec, address))
	if bankID < 0 || bankID >= spec.NumBanks {
		log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
	}

	return bankID
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

func bankSelectionAddress(spec Spec, addr uint64) uint64 {
	conversion := spec.BankAddressConversion
	if conversion.Kind == "" {
		return addr
	}
	converter := mem.InterleavingConverter{
		InterleavingSize:    conversion.InterleavingSize,
		TotalNumOfElements:  conversion.NumElements,
		CurrentElementIndex: conversion.ElementIndex,
		Offset:              conversion.Offset,
	}
	return converter.ConvertExternalToInternal(addr)
}
