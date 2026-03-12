package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// transactionSnapshot is the serializable representation of a transactionState.
type transactionSnapshot struct {
	ID                   string      `json:"id"`
	Action               int         `json:"action"`
	HasRead              bool        `json:"has_read"`
	ReadMsg              sim.MsgMeta `json:"read_msg"`
	HasWrite             bool        `json:"has_write"`
	WriteMsg             sim.MsgMeta `json:"write_msg"`
	HasFlush             bool        `json:"has_flush"`
	FlushMsg             sim.MsgMeta `json:"flush_msg"`
	FlushInvalidateAll   bool        `json:"flush_invalidate_all"`
	FlushDiscardInflight bool        `json:"flush_discard_inflight"`
	FlushPauseAfter      bool        `json:"flush_pause_after"`
	HasBlock             bool        `json:"has_block"`
	BlockSetID           int         `json:"block_set_id"`
	BlockWayID           int         `json:"block_way_id"`
	HasVictim            bool        `json:"has_victim"`
	VictimPID            uint32      `json:"victim_pid"`
	VictimTag            uint64      `json:"victim_tag"`
	VictimCacheAddress   uint64      `json:"victim_cache_address"`
	VictimDirtyMask      []bool      `json:"victim_dirty_mask"`
	FetchPID             uint32      `json:"fetch_pid"`
	FetchAddress         uint64      `json:"fetch_address"`
	FetchedData          []byte      `json:"fetched_data"`
	HasFetchReadReq      bool        `json:"has_fetch_read_req"`
	FetchReadReqMsg      sim.MsgMeta `json:"fetch_read_req_msg"`
	EvictingPID          uint32      `json:"evicting_pid"`
	EvictingAddr         uint64      `json:"evicting_addr"`
	EvictingData         []byte      `json:"evicting_data"`
	EvictingDirtyMask    []bool      `json:"evicting_dirty_mask"`
	HasEvictionWriteReq  bool        `json:"has_eviction_write_req"`
	EvictionWriteReqMsg  sim.MsgMeta `json:"eviction_write_req_msg"`
	MSHREntryIndex         int         `json:"mshr_entry_index"`
	HasMSHREntry           bool        `json:"has_mshr_entry"`
	MSHRData               []byte      `json:"mshr_data"`
	MSHRTransactionIndices []int       `json:"mshr_transaction_indices"`
}

// dirPipelineStageState captures one directory pipeline slot.
type dirPipelineStageState struct {
	Lane       int `json:"lane"`
	Stage      int `json:"stage"`
	TransIndex int `json:"trans_index"`
	CycleLeft  int `json:"cycle_left"`
}

// bankPipelineStageState captures one bank pipeline slot.
type bankPipelineStageState struct {
	Lane       int `json:"lane"`
	Stage      int `json:"stage"`
	TransIndex int `json:"trans_index"`
	CycleLeft  int `json:"cycle_left"`
}

// bankBufState wraps per-bank buffer indices to avoid nested slices.
type bankBufState struct {
	Indices []int `json:"indices"`
}

// bankPipelineState wraps per-bank pipeline stage states.
type bankPipelineState struct {
	Stages []bankPipelineStageState `json:"stages"`
}

// bankPostBufState wraps per-bank post-pipeline buffer indices.
type bankPostBufState struct {
	Indices []int `json:"indices"`
}

// flushReqState is a serializable representation of a cache.FlushReq.
type flushReqState struct {
	MsgMeta                 sim.MsgMeta `json:"msg_meta"`
	InvalidateAllCachelines bool        `json:"invalidate_all_cachelines"`
	DiscardInflight         bool        `json:"discard_inflight"`
	PauseAfterFlushing      bool        `json:"pause_after_flushing"`
}

func buildTransIndex(
	transactions []*transactionState,
) map[*transactionState]int {
	m := make(map[*transactionState]int, len(transactions))
	for i, t := range transactions {
		m[t] = i
	}

	return m
}

func snapshotTransaction(
	t *transactionState,
	lookup map[*transactionState]int,
) transactionSnapshot {
	s := transactionSnapshot{
		ID:             t.id,
		Action:         int(t.action),
		FetchPID:       uint32(t.fetchPID),
		FetchAddress:   t.fetchAddress,
		EvictingPID:    uint32(t.evictingPID),
		EvictingAddr:   t.evictingAddr,
		MSHREntryIndex: t.mshrEntryIndex,
		HasMSHREntry:   t.hasMSHREntry,
	}

	if t.read != nil {
		s.HasRead = true
		s.ReadMsg = t.read.MsgMeta
	}

	if t.write != nil {
		s.HasWrite = true
		s.WriteMsg = t.write.MsgMeta
	}

	if t.flush != nil {
		s.HasFlush = true
		s.FlushMsg = t.flush.MsgMeta
		s.FlushInvalidateAll = t.flush.InvalidateAllCachelines
		s.FlushDiscardInflight = t.flush.DiscardInflight
		s.FlushPauseAfter = t.flush.PauseAfterFlushing
	}

	if t.hasBlock {
		s.HasBlock = true
		s.BlockSetID = t.blockSetID
		s.BlockWayID = t.blockWayID
	}

	if t.hasVictim {
		s.HasVictim = true
		s.VictimPID = uint32(t.victimPID)
		s.VictimTag = t.victimTag
		s.VictimCacheAddress = t.victimCacheAddress
		if t.victimDirtyMask != nil {
			s.VictimDirtyMask = make([]bool, len(t.victimDirtyMask))
			copy(s.VictimDirtyMask, t.victimDirtyMask)
		}
	}

	if t.fetchedData != nil {
		s.FetchedData = make([]byte, len(t.fetchedData))
		copy(s.FetchedData, t.fetchedData)
	}

	if t.evictingData != nil {
		s.EvictingData = make([]byte, len(t.evictingData))
		copy(s.EvictingData, t.evictingData)
	}

	if t.evictingDirtyMask != nil {
		s.EvictingDirtyMask = make([]bool, len(t.evictingDirtyMask))
		copy(s.EvictingDirtyMask, t.evictingDirtyMask)
	}

	if t.fetchReadReq != nil {
		s.HasFetchReadReq = true
		s.FetchReadReqMsg = t.fetchReadReq.MsgMeta
	}

	if t.evictionWriteReq != nil {
		s.HasEvictionWriteReq = true
		s.EvictionWriteReqMsg = t.evictionWriteReq.MsgMeta
	}

	if t.mshrData != nil {
		s.MSHRData = make([]byte, len(t.mshrData))
		copy(s.MSHRData, t.mshrData)
	}

	if t.mshrTransactions != nil {
		s.MSHRTransactionIndices = make([]int, len(t.mshrTransactions))
		for i, mt := range t.mshrTransactions {
			if idx, ok := lookup[mt]; ok {
				s.MSHRTransactionIndices[i] = idx
			}
		}
	}

	return s
}

func snapshotAllTransactions(
	transactions []*transactionState,
	lookup map[*transactionState]int,
) []transactionSnapshot {
	states := make([]transactionSnapshot, len(transactions))

	for i, t := range transactions {
		states[i] = snapshotTransaction(t, lookup)
	}

	return states
}

func restoreAllTransactions(
	states []transactionSnapshot,
) []*transactionState {
	allTrans := make([]*transactionState, len(states))

	for i, s := range states {
		allTrans[i] = restoreTransactionCore(s)
	}

	// Second pass: resolve mshrTransactions pointers from saved indices
	for _, t := range allTrans {
		if t.mshrTransactionRestoreIndices != nil {
			t.mshrTransactions = make([]*transactionState, 0, len(t.mshrTransactionRestoreIndices))
			for _, idx := range t.mshrTransactionRestoreIndices {
				if idx >= 0 && idx < len(allTrans) {
					t.mshrTransactions = append(t.mshrTransactions, allTrans[idx])
				}
			}
			t.mshrTransactionRestoreIndices = nil
		}
	}

	return allTrans
}

func restoreTransactionCore(
	s transactionSnapshot,
) *transactionState {
	t := &transactionState{
		id:     s.ID,
		action: action(s.Action),
	}

	if s.HasRead {
		t.read = &mem.ReadReq{MsgMeta: s.ReadMsg}
	}

	if s.HasWrite {
		t.write = &mem.WriteReq{MsgMeta: s.WriteMsg}
	}

	if s.HasFlush {
		t.flush = &cache.FlushReq{
			MsgMeta:                 s.FlushMsg,
			InvalidateAllCachelines: s.FlushInvalidateAll,
			DiscardInflight:         s.FlushDiscardInflight,
			PauseAfterFlushing:      s.FlushPauseAfter,
		}
	}

	if s.HasBlock {
		t.hasBlock = true
		t.blockSetID = s.BlockSetID
		t.blockWayID = s.BlockWayID
	}

	if s.HasVictim {
		t.hasVictim = true
		t.victimPID = vm.PID(s.VictimPID)
		t.victimTag = s.VictimTag
		t.victimCacheAddress = s.VictimCacheAddress
		if s.VictimDirtyMask != nil {
			t.victimDirtyMask = make([]bool, len(s.VictimDirtyMask))
			copy(t.victimDirtyMask, s.VictimDirtyMask)
		}
	}

	t.fetchPID = vm.PID(s.FetchPID)
	t.fetchAddress = s.FetchAddress
	t.evictingPID = vm.PID(s.EvictingPID)
	t.evictingAddr = s.EvictingAddr

	if s.FetchedData != nil {
		t.fetchedData = make([]byte, len(s.FetchedData))
		copy(t.fetchedData, s.FetchedData)
	}

	if s.EvictingData != nil {
		t.evictingData = make([]byte, len(s.EvictingData))
		copy(t.evictingData, s.EvictingData)
	}

	if s.EvictingDirtyMask != nil {
		t.evictingDirtyMask = make([]bool, len(s.EvictingDirtyMask))
		copy(t.evictingDirtyMask, s.EvictingDirtyMask)
	}

	if s.HasFetchReadReq {
		t.fetchReadReq = &mem.ReadReq{MsgMeta: s.FetchReadReqMsg}
	}

	if s.HasEvictionWriteReq {
		t.evictionWriteReq = &mem.WriteReq{
			MsgMeta: s.EvictionWriteReqMsg,
		}
	}

	t.mshrEntryIndex = s.MSHREntryIndex
	t.hasMSHREntry = s.HasMSHREntry

	if s.MSHRData != nil {
		t.mshrData = make([]byte, len(s.MSHRData))
		copy(t.mshrData, s.MSHRData)
	}

	if s.MSHRTransactionIndices != nil {
		t.mshrTransactionRestoreIndices = make([]int, len(s.MSHRTransactionIndices))
		copy(t.mshrTransactionRestoreIndices, s.MSHRTransactionIndices)
	}

	return t
}

// --- Transaction list helpers ---

func snapshotTransList(
	list []*transactionState,
	lookup map[*transactionState]int,
) []int {
	indices := make([]int, len(list))
	for i, t := range list {
		indices[i] = lookup[t]
	}

	return indices
}

func restoreTransList(
	indices []int,
	allTrans []*transactionState,
) []*transactionState {
	list := make([]*transactionState, len(indices))
	for i, idx := range indices {
		list[i] = allTrans[idx]
	}

	return list
}

// --- Evicting list ---

func snapshotEvictingList(evictingList map[uint64]bool) map[uint64]bool {
	if len(evictingList) == 0 {
		return nil
	}

	out := make(map[uint64]bool, len(evictingList))
	for k, v := range evictingList {
		out[k] = v
	}

	return out
}

// --- MSHR stage ---

func snapshotMSHRStageEntry(
	ms *mshrStage,
	lookup map[*transactionState]int,
) (bool, int) {
	if !ms.hasProcessingTrans {
		return false, 0
	}

	if ms.processingTrans != nil {
		if idx, ok := lookup[ms.processingTrans]; ok {
			return true, idx
		}
	}

	return true, 0
}

// --- Flusher ---

func snapshotFlusherState(
	f *flusher,
) ([]blockRef, bool, flushReqState) {
	refs := make([]blockRef, len(f.blockToEvict))
	copy(refs, f.blockToEvict)

	if f.processingFlush == nil {
		return refs, false, flushReqState{}
	}

	return refs, true, flushReqState{
		MsgMeta:                 f.processingFlush.MsgMeta,
		InvalidateAllCachelines: f.processingFlush.InvalidateAllCachelines,
		DiscardInflight:         f.processingFlush.DiscardInflight,
		PauseAfterFlushing:      f.processingFlush.PauseAfterFlushing,
	}
}
