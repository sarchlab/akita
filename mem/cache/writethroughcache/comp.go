package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the writethroughcache.
type Spec struct {
	Freq                  timing.Freq `json:"freq"`
	NumReqPerCycle        int         `json:"num_req_per_cycle"`
	Log2BlockSize         uint64      `json:"log2_block_size"`
	BankLatency           int         `json:"bank_latency"`
	WayAssociativity      int         `json:"way_associativity"`
	MaxNumConcurrentTrans int         `json:"max_num_concurrent_trans"`
	NumBanks              int         `json:"num_banks"`
	NumMSHREntry          int         `json:"num_mshr_entry"`
	TotalByteSize         uint64      `json:"total_byte_size"`
	DirLatency            int         `json:"dir_latency"`

	// WritePolicyType selects the write-policy strategy.
	// Valid values: "write-around" (default, also used when empty),
	// "write-evict", "write-through".
	WritePolicyType string `json:"write_policy_type"`

	// AddressMapperType ("single" or "interleaved") and InterleavingSize
	// describe how to route to Resources.RemotePorts when no
	// Resources.AddressMapper is injected.
	AddressMapperType string `json:"address_mapper_type"`
	InterleavingSize  uint64 `json:"interleaving_size"`
}

// blockSize returns the size of a cache line, 2^Log2BlockSize bytes.
func (s Spec) blockSize() int {
	return 1 << s.Log2BlockSize
}

// numSets returns the number of sets in the directory,
// TotalByteSize / (WayAssociativity * 2^Log2BlockSize).
func (s Spec) numSets() int {
	return int(s.TotalByteSize / uint64(s.WayAssociativity*s.blockSize()))
}

// writePolicy returns the write policy, WritePolicyType, or "write-around" if
// it is empty.
func (s Spec) writePolicy() string {
	if s.WritePolicyType == "" {
		return "write-around"
	}

	return s.WritePolicyType
}

// Ports holds the writethroughcache's ports.
type Ports struct {
	// Top receives read and write requests from the upper level and returns
	// their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Bottom sends line fetches and written-through data to lower memory and
	// receives their responses.
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`

	// Control receives pause, drain, enable, reset, invalidate, and flush
	// commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// Middlewares holds the writethroughcache's behavior, run in field order every
// cycle. Control runs before the data pipeline so that a Pause, Drain, or
// Reset takes effect in the same cycle, before any Top or Bottom traffic
// advances.
type Middlewares struct {
	// Ctrl handles every control command: Pause, Drain, Enable, Reset,
	// Invalidate, and Flush.
	Ctrl *ctrlMiddleware

	// Pipeline runs the data pipeline: the intake, the directory, the banks,
	// the bottom parser, and the respond stage.
	Pipeline *pipelineMW
}

// State contains mutable runtime data for the writethroughcache.
type State struct {
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`

	// Transactions stores all transaction states as a flat list.
	Transactions []transactionState `json:"transactions"`

	DirBuf        queueing.Buffer[int]     `json:"dir_buf"`
	BankBufs      []queueing.Buffer[int]   `json:"bank_bufs"`
	DirPipeline   queueing.Pipeline[int]   `json:"dir_pipeline"`
	DirPostBuf    queueing.Buffer[int]     `json:"dir_post_buf"`
	BankPipelines []queueing.Pipeline[int] `json:"bank_pipelines"`
	BankPostBufs  []queueing.Buffer[int]   `json:"bank_post_bufs"`

	IsPaused      bool                 `json:"is_paused"`
	IsDraining    bool                 `json:"is_draining"`
	CurrentCmdID  uint64               `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort `json:"current_cmd_src"`
}

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

// transactionState is the canonical transaction type for the writethroughcache.
// All fields are directly JSON-serializable (no pointers to message types).
type transactionState struct {
	ID uint64 `json:"id"`

	// Read request fields (flattened from memprotocol.ReadReq)
	HasRead            bool              `json:"has_read"`
	ReadMeta           messaging.MsgMeta `json:"read_meta"`
	ReadAddress        uint64            `json:"read_address"`
	ReadAccessByteSize uint64            `json:"read_access_byte_size"`
	ReadPID            vm.PID            `json:"read_pid"`

	// ReadToBottom fields (flattened from memprotocol.ReadReq)
	HasReadToBottom  bool              `json:"has_read_to_bottom"`
	ReadToBottomMeta messaging.MsgMeta `json:"read_to_bottom_meta"`
	ReadToBottomPID  vm.PID            `json:"read_to_bottom_pid"`

	// Write request fields (flattened from memprotocol.WriteReq)
	HasWrite       bool              `json:"has_write"`
	WriteMeta      messaging.MsgMeta `json:"write_meta"`
	WriteAddress   uint64            `json:"write_address"`
	WriteData      []byte            `json:"write_data"`
	WriteDirtyMask []bool            `json:"write_dirty_mask"`
	WritePID       vm.PID            `json:"write_pid"`

	// WriteToBottom fields (flattened from memprotocol.WriteReq)
	HasWriteToBottom       bool              `json:"has_write_to_bottom"`
	WriteToBottomMeta      messaging.MsgMeta `json:"write_to_bottom_meta"`
	WriteToBottomPID       vm.PID            `json:"write_to_bottom_pid"`
	WriteToBottomData      []byte            `json:"write_to_bottom_data"`
	WriteToBottomDirtyMask []bool            `json:"write_to_bottom_dirty_mask"`

	BankAction            bankActionType `json:"bank_action"`
	BlockSetID            int            `json:"block_set_id"`
	BlockWayID            int            `json:"block_way_id"`
	HasBlock              bool           `json:"has_block"`
	Data                  []byte         `json:"data"`
	WriteFetchedDirtyMask []bool         `json:"write_fetched_dirty_mask"`

	FetchAndWrite   bool `json:"fetch_and_write"`
	Done            bool `json:"done"`
	BottomWriteDone bool `json:"bottom_write_done"`
	BankDone        bool `json:"bank_done"`

	// DirPipelineTaskID is the ID of the pipeline subtask that records the
	// transaction's traversal of the directory latency pipeline. It is a child
	// of the request's req_in task, opened when the transaction enters the
	// directory pipeline and closed when it leaves the post-pipeline buffer.
	DirPipelineTaskID uint64 `json:"dir_pipeline_task_id"`

	// BankTaskID is the ID of the pipeline subtask that records the
	// transaction's traversal of the bank (data-array) pipeline — the bank
	// read/write work. Like DirPipelineTaskID it is a child of the request's
	// req_in task, opened when the transaction is accepted into the bank
	// pipeline and closed when the bank finalizes the access.
	BankTaskID uint64 `json:"bank_task_id"`

	// WaitForMSHRFill / MSHRFillDone / MSHRFillFetcherIdx track an
	// MSHR-coalesced write whose data is merged into another transaction's
	// fetched line. The coalesced write never visits the bank itself, so
	// completion must wait until the fetcher's bankActionWriteFetched stage
	// has written the merged line into storage.
	WaitForMSHRFill    bool `json:"wait_for_mshr_fill"`
	MSHRFillDone       bool `json:"mshr_fill_done"`
	MSHRFillFetcherIdx int  `json:"mshr_fill_fetcher_idx"`

	// Removed marks a transaction that has been completed
	// and removed from active processing.
	Removed bool `json:"removed"`
}

func (t *transactionState) Address() uint64 {
	if t.HasRead {
		return t.ReadAddress
	}

	return t.WriteAddress
}

func (t *transactionState) PID() vm.PID {
	if t.HasRead {
		return t.ReadPID
	}

	return t.WritePID
}

// Resources holds the shared resources and external wiring referenced by the
// writethroughcache, supplied by the system builder. None of them is
// serialized with the component state.
type Resources struct {
	// Storage is the data array. It is required; size it to hold at least
	// Spec.TotalByteSize bytes.
	Storage *mem.Storage

	// AddressMapper routes requests to the lower-level modules. When it is
	// not supplied, the cache creates one from Spec.AddressMapperType over
	// RemotePorts. Only one of the two needs to be supplied.
	AddressMapper mem.AddressToPortMapper `json:"-"`
	RemotePorts   []messaging.RemotePort  `json:"-"`
}

// Comp is the writethroughcache component, a ticking component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
