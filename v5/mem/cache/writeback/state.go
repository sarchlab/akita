package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
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

	// mshrTransactions are resolved in a second pass by
	// resolveTransactionMSHRPointers, after all transactions are restored.
	// We store the raw indices temporarily in a helper field.
	if s.MSHRTransactionIndices != nil {
		t.mshrTransactionRestoreIndices = make([]int, len(s.MSHRTransactionIndices))
		copy(t.mshrTransactionRestoreIndices, s.MSHRTransactionIndices)
	}

	return t
}

// --- Deep copy helpers ---

func deepCopyDirectoryState(ds cache.DirectoryState) cache.DirectoryState {
	result := cache.DirectoryState{
		Sets: make([]cache.SetState, len(ds.Sets)),
	}

	for i, set := range ds.Sets {
		result.Sets[i] = cache.SetState{
			Blocks:   make([]cache.BlockState, len(set.Blocks)),
			LRUOrder: make([]int, len(set.LRUOrder)),
		}
		copy(result.Sets[i].Blocks, set.Blocks)
		copy(result.Sets[i].LRUOrder, set.LRUOrder)

		for j, b := range set.Blocks {
			if b.DirtyMask != nil {
				result.Sets[i].Blocks[j].DirtyMask = make([]bool, len(b.DirtyMask))
				copy(result.Sets[i].Blocks[j].DirtyMask, b.DirtyMask)
			}
		}
	}

	return result
}

func deepCopyMSHRState(ms cache.MSHRState) cache.MSHRState {
	result := cache.MSHRState{
		Entries: make([]cache.MSHREntryState, len(ms.Entries)),
	}

	for i, e := range ms.Entries {
		result.Entries[i] = e
		if e.TransactionIndices != nil {
			result.Entries[i].TransactionIndices = make([]int, len(e.TransactionIndices))
			copy(result.Entries[i].TransactionIndices, e.TransactionIndices)
		}
		if e.Data != nil {
			result.Entries[i].Data = make([]byte, len(e.Data))
			copy(result.Entries[i].Data, e.Data)
		}
	}

	return result
}

// --- Buffer snapshot/restore helpers ---

func snapshotDirBuf(
	buf queueing.Buffer,
	lookup map[*transactionState]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		indices[i] = lookup[e.(*transactionState)]
	}

	return indices
}

func restoreDirBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transactionState,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = allTrans[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotBankBufs(
	bankBufs []queueing.Buffer,
	lookup map[*transactionState]int,
) []bankBufState {
	result := make([]bankBufState, len(bankBufs))

	for i, buf := range bankBufs {
		elems := queueing.SnapshotBuffer(buf)
		indices := make([]int, len(elems))

		for j, e := range elems {
			indices[j] = lookup[e.(*transactionState)]
		}

		result[i] = bankBufState{Indices: indices}
	}

	return result
}

func restoreBankBufs(
	bankBufs []queueing.Buffer,
	states []bankBufState,
	allTrans []*transactionState,
) {
	for i, s := range states {
		elems := make([]interface{}, len(s.Indices))
		for j, idx := range s.Indices {
			elems[j] = allTrans[idx]
		}

		queueing.RestoreBuffer(bankBufs[i], elems)
	}
}

func snapshotMSHRStageBuf(
	buf queueing.Buffer,
	lookup map[*transactionState]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		indices[i] = lookup[e.(*transactionState)]
	}

	return indices
}

func restoreMSHRStageBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transactionState,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = allTrans[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotWriteBufferBuf(
	buf queueing.Buffer,
	lookup map[*transactionState]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		indices[i] = lookup[e.(*transactionState)]
	}

	return indices
}

func restoreWriteBufferBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transactionState,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = allTrans[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

// --- Pipeline snapshot/restore helpers ---

func snapshotDirPipeline(
	p queueing.Pipeline,
	lookup map[*transactionState]int,
) []dirPipelineStageState {
	snaps := queueing.SnapshotPipeline(p)
	states := make([]dirPipelineStageState, len(snaps))

	for i, s := range snaps {
		item := s.Elem.(dirPipelineItem)
		states[i] = dirPipelineStageState{
			Lane:       s.Lane,
			Stage:      s.Stage,
			TransIndex: lookup[item.trans],
			CycleLeft:  s.CycleLeft,
		}
	}

	return states
}

func restoreDirPipeline(
	p queueing.Pipeline,
	states []dirPipelineStageState,
	allTrans []*transactionState,
) {
	snaps := make([]queueing.PipelineStageSnapshot, len(states))

	for i, s := range states {
		snaps[i] = queueing.PipelineStageSnapshot{
			Lane:  s.Lane,
			Stage: s.Stage,
			Elem: dirPipelineItem{
				trans: allTrans[s.TransIndex],
			},
			CycleLeft: s.CycleLeft,
		}
	}

	queueing.RestorePipeline(p, snaps)
}

func snapshotDirPostBuf(
	buf queueing.Buffer,
	lookup map[*transactionState]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		item := e.(dirPipelineItem)
		indices[i] = lookup[item.trans]
	}

	return indices
}

func restoreDirPostBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transactionState,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = dirPipelineItem{trans: allTrans[idx]}
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotBankPipelines(
	bankStages []*bankStage,
	lookup map[*transactionState]int,
) []bankPipelineState {
	result := make([]bankPipelineState, len(bankStages))

	for i, bs := range bankStages {
		snaps := queueing.SnapshotPipeline(bs.pipeline)
		states := make([]bankPipelineStageState, len(snaps))

		for j, s := range snaps {
			elem := s.Elem.(bankPipelineElem)
			states[j] = bankPipelineStageState{
				Lane:       s.Lane,
				Stage:      s.Stage,
				TransIndex: lookup[elem.trans],
				CycleLeft:  s.CycleLeft,
			}
		}

		result[i] = bankPipelineState{Stages: states}
	}

	return result
}

func restoreBankPipelines(
	bankStages []*bankStage,
	pipeStates []bankPipelineState,
	allTrans []*transactionState,
) {
	for i, ps := range pipeStates {
		snaps := make(
			[]queueing.PipelineStageSnapshot, len(ps.Stages))

		for j, s := range ps.Stages {
			snaps[j] = queueing.PipelineStageSnapshot{
				Lane:  s.Lane,
				Stage: s.Stage,
				Elem: bankPipelineElem{
					trans: allTrans[s.TransIndex],
				},
				CycleLeft: s.CycleLeft,
			}
		}

		queueing.RestorePipeline(bankStages[i].pipeline, snaps)
	}
}

func snapshotBankPostBufs(
	bankStages []*bankStage,
	lookup map[*transactionState]int,
) []bankPostBufState {
	result := make([]bankPostBufState, len(bankStages))

	for i, bs := range bankStages {
		elems := queueing.SnapshotBuffer(bs.postPipelineBuf)
		indices := make([]int, len(elems))

		for j, e := range elems {
			elem := e.(bankPipelineElem)
			indices[j] = lookup[elem.trans]
		}

		result[i] = bankPostBufState{Indices: indices}
	}

	return result
}

func restoreBankPostBufs(
	bankStages []*bankStage,
	states []bankPostBufState,
	allTrans []*transactionState,
) {
	for i, s := range states {
		elems := make([]interface{}, len(s.Indices))
		for j, idx := range s.Indices {
			elems[j] = bankPipelineElem{
				trans: allTrans[idx],
			}
		}

		queueing.RestoreBuffer(
			bankStages[i].postPipelineBuf, elems)
	}
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

// --- Bank counters ---

func snapshotBankCounters(
	bankStages []*bankStage,
) (inflightCounts, downwardCounts []int) {
	inflightCounts = make([]int, len(bankStages))
	downwardCounts = make([]int, len(bankStages))

	for i, bs := range bankStages {
		inflightCounts[i] = bs.inflightTransCount
		downwardCounts[i] = bs.downwardInflightTransCount
	}

	return inflightCounts, downwardCounts
}

// --- Write buffer stage ---

func snapshotWriteBufferStage(
	wb *writeBufferStage,
	lookup map[*transactionState]int,
) (pending, fetchInfl, evictInfl []int) {
	pending = snapshotTransList(wb.pendingEvictions, lookup)
	fetchInfl = snapshotTransList(wb.inflightFetch, lookup)
	evictInfl = snapshotTransList(wb.inflightEviction, lookup)

	return pending, fetchInfl, evictInfl
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

// --- State snapshot/restore on middleware ---

func (m *middleware) snapshotState() State {
	lookup := buildTransIndex(m.inFlightTransactions)

	s := State{
		CacheState:   int(m.state),
		EvictingList: snapshotEvictingList(m.evictingList),
	}

	// Deep copy directory and MSHR state
	s.DirectoryState = deepCopyDirectoryState(m.directoryState)
	s.MSHRState = deepCopyMSHRState(m.mshrState)

	s.Transactions = snapshotAllTransactions(
		m.inFlightTransactions, lookup)

	s.DirStageBufIndices = snapshotDirBuf(m.dirStageBuffer, lookup)
	s.DirToBankBufIndices = snapshotBankBufs(m.dirToBankBuffers, lookup)
	s.WriteBufferToBankBufIndices = snapshotBankBufs(
		m.writeBufferToBankBuffers, lookup)
	s.MSHRStageBufEntries = snapshotMSHRStageBuf(m.mshrStageBuffer, lookup)
	s.WriteBufferBufIndices = snapshotWriteBufferBuf(
		m.writeBufferBuffer, lookup)

	s.DirPipelineStages = snapshotDirPipeline(
		m.dirStage.pipeline, lookup)
	s.DirPostPipelineBufIndices = snapshotDirPostBuf(
		m.dirStage.buf, lookup)
	s.BankPipelineStages = snapshotBankPipelines(m.bankStages, lookup)
	s.BankPostPipelineBufIndices = snapshotBankPostBufs(
		m.bankStages, lookup)

	s.BankInflightTransCounts, s.BankDownwardInflightTransCounts =
		snapshotBankCounters(m.bankStages)
	s.PendingEvictionIndices, s.InflightFetchIndices,
		s.InflightEvictionIndices =
		snapshotWriteBufferStage(m.writeBuffer, lookup)
	s.HasProcessingMSHREntry, s.ProcessingMSHREntryIdx =
		snapshotMSHRStageEntry(m.mshrStage, lookup)
	s.FlusherBlockToEvictRefs, s.HasProcessingFlush,
		s.ProcessingFlush = snapshotFlusherState(m.flusher)

	return s
}

func (m *middleware) restoreFromState(s State) {
	m.state = cacheState(s.CacheState)

	m.directoryState = deepCopyDirectoryState(s.DirectoryState)
	m.mshrState = deepCopyMSHRState(s.MSHRState)

	allTrans := restoreAllTransactions(s.Transactions)
	m.inFlightTransactions = allTrans

	// Evicting list
	m.evictingList = make(map[uint64]bool)
	for k, v := range s.EvictingList {
		m.evictingList[k] = v
	}

	// Restore buffers and pipelines
	restoreDirBuf(m.dirStageBuffer, s.DirStageBufIndices, allTrans)
	restoreBankBufs(m.dirToBankBuffers,
		s.DirToBankBufIndices, allTrans)
	restoreBankBufs(m.writeBufferToBankBuffers,
		s.WriteBufferToBankBufIndices, allTrans)
	restoreMSHRStageBuf(m.mshrStageBuffer, s.MSHRStageBufEntries, allTrans)
	restoreWriteBufferBuf(m.writeBufferBuffer,
		s.WriteBufferBufIndices, allTrans)

	restoreDirPipeline(
		m.dirStage.pipeline, s.DirPipelineStages, allTrans)
	restoreDirPostBuf(
		m.dirStage.buf, s.DirPostPipelineBufIndices, allTrans)
	restoreBankPipelines(
		m.bankStages, s.BankPipelineStages, allTrans)
	restoreBankPostBufs(
		m.bankStages, s.BankPostPipelineBufIndices, allTrans)

	// Bank counters
	for i, bs := range m.bankStages {
		if i < len(s.BankInflightTransCounts) {
			bs.inflightTransCount = s.BankInflightTransCounts[i]
		}

		if i < len(s.BankDownwardInflightTransCounts) {
			bs.downwardInflightTransCount =
				s.BankDownwardInflightTransCounts[i]
		}
	}

	// Write buffer stage
	m.writeBuffer.pendingEvictions = restoreTransList(
		s.PendingEvictionIndices, allTrans)
	m.writeBuffer.inflightFetch = restoreTransList(
		s.InflightFetchIndices, allTrans)
	m.writeBuffer.inflightEviction = restoreTransList(
		s.InflightEvictionIndices, allTrans)

	// MSHR stage
	m.mshrStage.hasProcessingTrans = s.HasProcessingMSHREntry
	if s.HasProcessingMSHREntry && s.ProcessingMSHREntryIdx >= 0 && s.ProcessingMSHREntryIdx < len(allTrans) {
		trans := allTrans[s.ProcessingMSHREntryIdx]
		m.mshrStage.processingTrans = trans
		m.mshrStage.processingTransList = trans.mshrTransactions
		m.mshrStage.processingData = trans.mshrData
	}

	// Flusher
	m.flusher.blockToEvict = make([]blockRef, len(s.FlusherBlockToEvictRefs))
	copy(m.flusher.blockToEvict, s.FlusherBlockToEvictRefs)
	m.flusher.processingFlush = nil

	if s.HasProcessingFlush {
		m.flusher.processingFlush = &cache.FlushReq{
			MsgMeta:                 s.ProcessingFlush.MsgMeta,
			InvalidateAllCachelines: s.ProcessingFlush.InvalidateAllCachelines,
			DiscardInflight:         s.ProcessingFlush.DiscardInflight,
			PauseAfterFlushing:      s.ProcessingFlush.PauseAfterFlushing,
		}
	}
}

// GetState converts runtime mutable data into a serializable State.
func (m *middleware) GetState() State {
	state := m.snapshotState()
	m.comp.SetState(state)

	return state
}

// SetState restores runtime mutable data from a serializable State.
func (m *middleware) SetState(state State) {
	m.comp.SetState(state)
	m.restoreFromState(state)
}
