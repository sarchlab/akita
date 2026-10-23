package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type cacheState int

const (
	cacheStateInvalid cacheState = iota
	cacheStateRunning
	cacheStatePreFlushing
	cacheStateFlushing
	cacheStatePaused
	cacheStateDraining
)

// Spec contains immutable configuration for the writeback cache.
type Spec struct {
	// Lower memory routing configuration.
	Bottom mem.AddressMapper `json:"bottom"`

	Freq           timing.Freq `json:"freq"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`
	Log2BlockSize  uint64      `json:"log2_block_size"`
	BankLatency    int         `json:"bank_latency"`
	NumWays        int         `json:"num_ways"`

	// NumBanks is the number of banks and must be positive.
	NumBanks             int `json:"num_banks"`
	NumMSHREntries       int `json:"num_mshr_entries"`
	NumSets              int `json:"num_sets"`
	DirLatency           int `json:"dir_latency"`
	WriteBufferCapacity  int `json:"write_buffer_capacity"`
	MaxInFlightFetches   int `json:"max_in_flight_fetches"`
	MaxInFlightEvictions int `json:"max_in_flight_evictions"`
	// Pipeline widths are entries per cycle. Internal output queues hold one
	// cycle of their pipeline width; other stage queues use NumReqPerCycle.
	DirectoryWidth int `json:"directory_width"`
	BankWidth      int `json:"bank_width"`
}

// ByteSize reports the capacity implied by the configured geometry.
// It is an observation, not a second configurable capacity.
func (s Spec) ByteSize() uint64 {
	return uint64(s.NumSets) * uint64(s.NumWays) * (uint64(1) << s.Log2BlockSize)
}

// Ports holds the writeback cache's ports.
type Ports struct {
	// Top receives read and write requests from the upper level and returns
	// their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Bottom sends line fetches and eviction write-backs to lower memory and
	// receives their responses.
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`

	// Control receives pause, drain, enable, reset, invalidate, and flush
	// commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the writeback cache's behavior, run in field order every
// cycle. Control runs before the data pipeline so that a Pause, Drain, or
// Reset takes effect in the same cycle, before any Top or Bottom traffic or
// in-flight operation advances.
type middlewares struct {
	// Ctrl handles every control command except Flush: Pause, Drain, Enable,
	// Reset, and Invalidate.
	Ctrl *ctrlMiddleware

	// Flusher handles Flush commands, writing dirty blocks back to lower
	// memory.
	Flusher *controlMW

	// Pipeline runs the data pipeline: the top parser, the directory, the
	// banks, the MSHR stage, and the write buffer.
	Pipeline *pipelineMW
}

// state contains mutable runtime data for the writeback cache.
type state struct {
	CacheState     int                  `json:"cache_state"`
	CurrentCmdID   uint64               `json:"current_cmd_id"`
	CurrentCmdSrc  messaging.RemotePort `json:"current_cmd_src"`
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`
	Transactions   []transactionState   `json:"transactions"`
	EvictingList   map[uint64]bool      `json:"evicting_list"`

	// Buffers (transaction indices stored as int)
	DirStageBuf           queueing.Buffer[int]   `json:"dir_stage_buf"`
	DirToBankBufs         []queueing.Buffer[int] `json:"dir_to_bank_bufs"`
	WriteBufferToBankBufs []queueing.Buffer[int] `json:"write_buffer_to_bank_bufs"`
	MSHRStageBuf          queueing.Buffer[int]   `json:"mshr_stage_buf"`
	WriteBufferBuf        queueing.Buffer[int]   `json:"write_buffer_buf"`

	// Directory pipeline + post-buf
	DirPipeline        queueing.Pipeline[int] `json:"dir_pipeline"`
	DirPostPipelineBuf queueing.Buffer[int]   `json:"dir_post_pipeline_buf"`

	// Bank pipeline + post-buf + counters
	BankPipelines                   []queueing.Pipeline[int] `json:"bank_pipelines"`
	BankPostPipelineBufs            []queueing.Buffer[int]   `json:"bank_post_pipeline_bufs"`
	BankInflightTransCounts         []int                    `json:"bank_inflight_trans_counts"`
	BankDownwardInflightTransCounts []int                    `json:"bank_downward_inflight_trans_counts"`

	// Write buffer stage
	PendingEvictionIndices  []int `json:"pending_eviction_indices"`
	InflightFetchIndices    []int `json:"inflight_fetch_indices"`
	InflightEvictionIndices []int `json:"inflight_eviction_indices"`

	// MSHR stage
	HasProcessingMSHREntry bool `json:"has_processing_mshr_entry"`
	ProcessingMSHREntryIdx int  `json:"processing_mshr_entry_idx"`

	// Flusher
	FlusherBlockToEvictRefs []blockRef    `json:"flusher_block_to_evict_refs"`
	HasProcessingFlush      bool          `json:"has_processing_flush"`
	ProcessingFlush         flushReqState `json:"processing_flush"`
}

// allocTransaction stores t in the Transactions slice and returns its index.
// Completed transactions are marked Removed but their slots are never deleted
// (other stages hold indices into the slice via the various buffers and the
// MSHR, so indices must stay stable). Reuse a Removed slot when one is
// available so the slice stays bounded by the number of active transactions
// instead of growing with every request ever issued.
func (s *state) allocTransaction(t transactionState) int {
	for i := range s.Transactions {
		if !s.Transactions[i].Removed {
			continue
		}

		// The MSHR stage drains the waiter list of the transaction at
		// ProcessingMSHREntryIdx across multiple ticks. That owner
		// transaction is itself one of its waiters, so it is commonly marked
		// Removed before the list is fully drained — yet its
		// MSHRTransactionIndices must stay intact until the stage releases it.
		// Skip its slot so a later top request in the same tick (topParser
		// runs after mshrStage) cannot overwrite the pending waiter list.
		if s.HasProcessingMSHREntry && i == s.ProcessingMSHREntryIdx {
			continue
		}

		// Do not reuse a slot still referenced by an in-flight bottom-port
		// transaction. Eviction write-backs and line fetches are matched to
		// their lower-memory responses by the request ID stored in the
		// transaction slot; overwriting the slot before the response returns
		// makes that response an unrecognized "orphan" (silently dropped) and
		// leaves the eviction/fetch permanently in the in-flight list, so a
		// later Flush/Drain never reaches quiescence and the simulation
		// deadlocks. A slot can be both Removed and in-flight: e.g.
		// writeBufferEvictAndWrite reuses one transaction for a bank write-hit
		// (which marks it Removed) and the victim write-back (which stays in
		// InflightEvictionIndices until its WriteDoneRsp lands).
		if s.indexHasInflightBottomTransaction(i) {
			continue
		}

		s.Transactions[i] = t
		return i
	}

	s.Transactions = append(s.Transactions, t)
	return len(s.Transactions) - 1
}

// indexHasInflightBottomTransaction reports whether transaction slot i is still
// referenced by a pending or in-flight bottom-port transaction (an eviction
// write-back or a line fetch). Such slots must not be reused, because the
// matching bottom-port response is correlated by the request ID held in the
// slot.
func (s *state) indexHasInflightBottomTransaction(i int) bool {
	for _, idx := range s.InflightEvictionIndices {
		if idx == i {
			return true
		}
	}

	for _, idx := range s.PendingEvictionIndices {
		if idx == i {
			return true
		}
	}

	for _, idx := range s.InflightFetchIndices {
		if idx == i {
			return true
		}
	}

	return false
}

// flushReqState is a serializable representation of a flush control request.
type flushReqState struct {
	RecvTaskID uint64        `json:"recv_task_id"`
	Msg        messaging.Msg `json:"msg_meta"`

	// FilterAddresses / FilterPID narrow which dirty blocks are written
	// back. An empty address list matches every block address; a zero PID
	// matches every PID (same convention as Invalidate).
	FilterAddresses []uint64 `json:"filter_addresses"`
	FilterPID       vm.PID   `json:"filter_pid"`

	// FlushedRefs records the blocks selected for write-back so that, once
	// the write-backs complete, exactly those blocks (and no others) are
	// marked clean. Blocks outside the filter are left untouched.
	FlushedRefs []blockRef `json:"flushed_refs"`
}

type action int

const (
	actionInvalid action = iota
	bankReadHit
	bankWriteHit
	bankEvict
	bankEvictAndWrite
	bankEvictAndFetch
	bankWriteFetched
	writeBufferFetch
	writeBufferEvictAndFetch
	writeBufferEvictAndWrite
	writeBufferFlush
)

// transactionState is the canonical runtime transaction type.
// All fields are flat and directly JSON-serializable.
type transactionState struct {
	RecvTaskID     uint64 `json:"recv_task_id"`
	FetchTaskID    uint64 `json:"fetch_task_id"`
	EvictionTaskID uint64 `json:"eviction_task_id"`

	Action action `json:"action"`

	ID uint64 `json:"id"`

	// Read request fields (flat, replaces memprotocol.ReadReq)
	HasRead            bool          `json:"has_read"`
	ReadMeta           messaging.Msg `json:"read_meta"`
	ReadAddress        uint64        `json:"read_address"`
	ReadAccessByteSize uint64        `json:"read_access_byte_size"`
	ReadPID            vm.PID        `json:"read_pid"`

	// Write request fields (flat, replaces memprotocol.WriteReq)
	HasWrite       bool          `json:"has_write"`
	WriteMeta      messaging.Msg `json:"write_meta"`
	WriteAddress   uint64        `json:"write_address"`
	WriteData      []byte        `json:"write_data"`
	WriteDirtyMask []bool        `json:"write_dirty_mask"`
	WritePID       vm.PID        `json:"write_pid"`

	// Flush request fields (flat)
	HasFlush  bool          `json:"has_flush"`
	FlushMeta messaging.Msg `json:"flush_meta"`

	// Block reference (into directoryState)
	BlockSetID int  `json:"block_set_id"`
	BlockWayID int  `json:"block_way_id"`
	HasBlock   bool `json:"has_block"`

	// Victim data (inlined, not a pointer to cache.Block)
	VictimPID          vm.PID `json:"victim_pid"`
	VictimTag          uint64 `json:"victim_tag"`
	VictimCacheAddress uint64 `json:"victim_cache_address"`
	VictimDirtyMask    []bool `json:"victim_dirty_mask"`
	HasVictim          bool   `json:"has_victim"`

	FetchPID     vm.PID `json:"fetch_pid"`
	FetchAddress uint64 `json:"fetch_address"`
	FetchedData  []byte `json:"fetched_data"`

	// Fetch read request fields (flat, replaces memprotocol.ReadReq)
	HasFetchReadReq  bool          `json:"has_fetch_read_req"`
	FetchReadReqMeta messaging.Msg `json:"fetch_read_req_meta"`

	EvictingPID       vm.PID `json:"evicting_pid"`
	EvictingAddr      uint64 `json:"evicting_addr"`
	EvictingData      []byte `json:"evicting_data"`
	EvictingDirtyMask []bool `json:"evicting_dirty_mask"`

	// Eviction write request fields (flat, replaces memprotocol.WriteReq)
	HasEvictionWriteReq  bool          `json:"has_eviction_write_req"`
	EvictionWriteReqMeta messaging.Msg `json:"eviction_write_req_meta"`

	// DirPipelinePID is the tracing task ID of the directory-pipeline subtask
	// (a child of the request's req_in). It is set when the transaction enters
	// the directory pipeline stage and consumed when the transaction leaves the
	// directory post-pipeline buffer, so the retrieve->directory latency gap is
	// attributed to a pipeline subtask.
	DirPipelinePID uint64 `json:"dir_pipeline_pid"`

	// BankPID is the tracing task ID of the bank (data-array) pipeline subtask
	// (a child of the request's req_in). It is set when the transaction is
	// accepted into the bank pipeline and consumed when the bank finalizes the
	// access, pairing with the ".bank" work milestone so the bank latency shows
	// as a child bar instead of an unexplained tail before the response.
	BankPID uint64 `json:"bank_pid"`

	// MSHR entry reference (into mshrState.Entries)
	MSHREntryIndex int  `json:"mshr_entry_index"`
	HasMSHREntry   bool `json:"has_mshr_entry"`

	// Data saved from MSHR entry before removal (for bank/mshr stage)
	MSHRData               []byte `json:"mshr_data"`
	MSHRTransactionIndices []int  `json:"mshr_transaction_indices"`

	// Removed marks this transaction slot as logically deleted.
	Removed bool `json:"removed"`
}

// accessReqAddress returns the address of the access request.
func (t *transactionState) accessReqAddress() uint64 {
	if t.HasRead {
		return t.ReadAddress
	}
	if t.HasWrite {
		return t.WriteAddress
	}
	panic("no access request")
}

// hasReqMeta reports whether the transaction carries a primary request, i.e.
// whether there is a req_in task to attribute work to. Every transaction that
// reaches the bank in a real run has one (it passed
// through the directory); it guards against synthetic bank-only transactions.
func (t *transactionState) hasReqMeta() bool {
	return t.HasRead || t.HasWrite || t.HasFlush
}

// Resources holds live shared storage; routing configuration belongs to Spec.
type Resources struct{ Storage *mem.Storage }

// Comp is the writeback cache component, a ticking component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
