package cache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// BlockState is a serializable representation of a cache Block.
type BlockState struct {
	PID          uint32 `json:"pid"`
	Tag          uint64 `json:"tag"`
	WayID        int    `json:"way_id"`
	SetID        int    `json:"set_id"`
	CacheAddress uint64 `json:"cache_address"`
	IsValid      bool   `json:"is_valid"`
	IsDirty      bool   `json:"is_dirty"`
	ReadCount    int    `json:"read_count"`
	IsLocked     bool   `json:"is_locked"`
	DirtyMask    []bool `json:"dirty_mask"`
}

// SetState is a serializable representation of a cache Set.
type SetState struct {
	Blocks   []BlockState `json:"blocks"`
	LRUOrder []int        `json:"lru_order"`
}

// DirectoryState is a serializable representation of a DirectoryImpl.
type DirectoryState struct {
	Sets []SetState `json:"sets"`
}

// MSHREntryState is a serializable representation of an MSHREntry.
type MSHREntryState struct {
	PID                uint32        `json:"pid"`
	Address            uint64        `json:"address"`
	TransactionIndices []int         `json:"transaction_indices"`
	BlockSetID         int           `json:"block_set_id"`
	BlockWayID         int           `json:"block_way_id"`
	HasBlock           bool          `json:"has_block"`
	HasReadReq         bool          `json:"has_read_req"`
	ReadReq            messaging.Msg `json:"read_req"`
	HasDataReady       bool          `json:"has_data_ready"`
	DataReady          messaging.Msg `json:"data_ready"`
	Data               []byte        `json:"data"`
}

// Matches reports whether the MSHR entry has the given PID and address.
func (e MSHREntryState) Matches(pid vm.PID, addr uint64) bool {
	return vm.PID(e.PID) == pid && e.Address == addr
}

// MSHRState is a serializable representation of an mshrImpl.
type MSHRState struct {
	Entries []MSHREntryState `json:"entries"`
}
