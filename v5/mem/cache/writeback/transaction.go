package writeback

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

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
	Action action `json:"action"`

	ID string `json:"id"`

	// Read request fields (flat, replaces *mem.ReadReq)
	HasRead          bool         `json:"has_read"`
	ReadMeta         sim.MsgMeta  `json:"read_meta"`
	ReadAddress      uint64       `json:"read_address"`
	ReadAccessByteSize uint64     `json:"read_access_byte_size"`
	ReadPID          vm.PID       `json:"read_pid"`

	// Write request fields (flat, replaces *mem.WriteReq)
	HasWrite       bool     `json:"has_write"`
	WriteMeta      sim.MsgMeta `json:"write_meta"`
	WriteAddress   uint64   `json:"write_address"`
	WriteData      []byte   `json:"write_data"`
	WriteDirtyMask []bool   `json:"write_dirty_mask"`
	WritePID       vm.PID   `json:"write_pid"`

	// Flush request fields (flat, replaces *cache.FlushReq)
	HasFlush             bool        `json:"has_flush"`
	FlushMeta            sim.MsgMeta `json:"flush_meta"`
	FlushInvalidateAll   bool        `json:"flush_invalidate_all"`
	FlushDiscardInflight bool        `json:"flush_discard_inflight"`
	FlushPauseAfter      bool        `json:"flush_pause_after"`

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

	// Fetch read request fields (flat, replaces *mem.ReadReq)
	HasFetchReadReq  bool        `json:"has_fetch_read_req"`
	FetchReadReqMeta sim.MsgMeta `json:"fetch_read_req_meta"`

	EvictingPID       vm.PID `json:"evicting_pid"`
	EvictingAddr      uint64 `json:"evicting_addr"`
	EvictingData      []byte `json:"evicting_data"`
	EvictingDirtyMask []bool `json:"evicting_dirty_mask"`

	// Eviction write request fields (flat, replaces *mem.WriteReq)
	HasEvictionWriteReq  bool        `json:"has_eviction_write_req"`
	EvictionWriteReqMeta sim.MsgMeta `json:"eviction_write_req_meta"`

	// MSHR entry reference (into mshrState.Entries)
	MSHREntryIndex int  `json:"mshr_entry_index"`
	HasMSHREntry   bool `json:"has_mshr_entry"`

	// Data saved from MSHR entry before removal (for bank/mshr stage)
	MSHRData                 []byte `json:"mshr_data"`
	MSHRTransactionIndices   []int  `json:"mshr_transaction_indices"`

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


// reqMeta returns the MsgMeta of the primary request.
func (t *transactionState) reqMeta() sim.MsgMeta {
	if t.HasRead {
		return t.ReadMeta
	}
	if t.HasWrite {
		return t.WriteMeta
	}
	if t.HasFlush {
		return t.FlushMeta
	}
	panic("no request")
}
