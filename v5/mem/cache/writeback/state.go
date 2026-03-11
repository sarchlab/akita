package writeback

import (
	"io"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// transactionState is the serializable representation of a transaction.
type transactionState struct {
	ID                  string       `json:"id"`
	Action              int          `json:"action"`
	HasRead             bool         `json:"has_read"`
	ReadMsg             sim.MsgMeta  `json:"read_msg"`
	HasWrite            bool         `json:"has_write"`
	WriteMsg            sim.MsgMeta  `json:"write_msg"`
	HasFlush            bool         `json:"has_flush"`
	FlushMsg            sim.MsgMeta  `json:"flush_msg"`
	FlushInvalidateAll  bool         `json:"flush_invalidate_all"`
	FlushDiscardInflight bool        `json:"flush_discard_inflight"`
	FlushPauseAfter     bool         `json:"flush_pause_after"`
	HasBlock            bool         `json:"has_block"`
	BlockSetID          int          `json:"block_set_id"`
	BlockWayID          int          `json:"block_way_id"`
	HasVictim           bool         `json:"has_victim"`
	Victim              cache.BlockState `json:"victim"`
	FetchPID            uint32       `json:"fetch_pid"`
	FetchAddress        uint64       `json:"fetch_address"`
	FetchedData         []byte       `json:"fetched_data"`
	HasFetchReadReq     bool         `json:"has_fetch_read_req"`
	FetchReadReqMsg     sim.MsgMeta  `json:"fetch_read_req_msg"`
	EvictingPID         uint32       `json:"evicting_pid"`
	EvictingAddr        uint64       `json:"evicting_addr"`
	EvictingData        []byte       `json:"evicting_data"`
	EvictingDirtyMask   []bool       `json:"evicting_dirty_mask"`
	HasEvictionWriteReq bool         `json:"has_eviction_write_req"`
	EvictionWriteReqMsg sim.MsgMeta  `json:"eviction_write_req_msg"`
	MSHREntryIndex      int          `json:"mshr_entry_index"`
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

// blockRef is a set+way pair referencing a block in the directory.
type blockRef struct {
	SetID int `json:"set_id"`
	WayID int `json:"way_id"`
}

// flushReqState is a serializable representation of a cache.FlushReq.
type flushReqState struct {
	MsgMeta                sim.MsgMeta `json:"msg_meta"`
	InvalidateAllCachelines bool       `json:"invalidate_all_cachelines"`
	DiscardInflight         bool       `json:"discard_inflight"`
	PauseAfterFlushing      bool       `json:"pause_after_flushing"`
}

func buildTransIndex(
	transactions []*transaction,
) map[*transaction]int {
	m := make(map[*transaction]int, len(transactions))
	for i, t := range transactions {
		m[t] = i
	}

	return m
}

func buildMSHREntryLookup(
	mshr cache.MSHR,
) map[*cache.MSHREntry]int {
	entries := mshr.AllEntries()
	m := make(map[*cache.MSHREntry]int, len(entries))

	for i, e := range entries {
		m[e] = i
	}

	return m
}

func snapshotTransaction(
	t *transaction,
	mshrLookup map[*cache.MSHREntry]int,
) transactionState {
	s := transactionState{
		ID:             t.id,
		Action:         int(t.action),
		FetchPID:       uint32(t.fetchPID),
		FetchAddress:   t.fetchAddress,
		EvictingPID:    uint32(t.evictingPID),
		EvictingAddr:   t.evictingAddr,
		MSHREntryIndex: -1,
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

	if t.block != nil {
		s.HasBlock = true
		s.BlockSetID = t.block.SetID
		s.BlockWayID = t.block.WayID
	}

	snapshotVictim(t, &s)
	snapshotTransData(t, &s)

	if t.fetchReadReq != nil {
		s.HasFetchReadReq = true
		s.FetchReadReqMsg = t.fetchReadReq.MsgMeta
	}

	if t.evictionWriteReq != nil {
		s.HasEvictionWriteReq = true
		s.EvictionWriteReqMsg = t.evictionWriteReq.MsgMeta
	}

	if t.mshrEntry != nil {
		if idx, ok := mshrLookup[t.mshrEntry]; ok {
			s.MSHREntryIndex = idx
		}
	}

	return s
}

func snapshotVictim(t *transaction, s *transactionState) {
	if t.victim == nil {
		return
	}

	s.HasVictim = true
	s.Victim = cache.BlockState{
		PID:          uint32(t.victim.PID),
		Tag:          t.victim.Tag,
		WayID:        t.victim.WayID,
		SetID:        t.victim.SetID,
		CacheAddress: t.victim.CacheAddress,
		IsValid:      t.victim.IsValid,
		IsDirty:      t.victim.IsDirty,
		ReadCount:    t.victim.ReadCount,
		IsLocked:     t.victim.IsLocked,
	}

	if t.victim.DirtyMask != nil {
		s.Victim.DirtyMask = make([]bool, len(t.victim.DirtyMask))
		copy(s.Victim.DirtyMask, t.victim.DirtyMask)
	}
}

func snapshotTransData(t *transaction, s *transactionState) {
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
}

func snapshotAllTransactions(
	transactions []*transaction,
	mshrLookup map[*cache.MSHREntry]int,
) []transactionState {
	states := make([]transactionState, len(transactions))

	for i, t := range transactions {
		states[i] = snapshotTransaction(t, mshrLookup)
	}

	return states
}

func restoreAllTransactions(
	states []transactionState,
	dir cache.Directory,
	mshrEntries []*cache.MSHREntry,
) []*transaction {
	allTrans := make([]*transaction, len(states))

	for i, s := range states {
		allTrans[i] = restoreTransactionCore(s, dir, mshrEntries)
	}

	return allTrans
}

func restoreTransactionCore(
	s transactionState,
	dir cache.Directory,
	mshrEntries []*cache.MSHREntry,
) *transaction {
	t := &transaction{
		id:     s.ID,
		action: action(s.Action),
	}

	restoreTransMsgs(t, s)
	restoreTransBlock(t, s, dir)
	restoreTransVictim(t, s)
	restoreTransData(t, s)
	restoreTransFetchEvict(t, s)

	if s.MSHREntryIndex >= 0 && s.MSHREntryIndex < len(mshrEntries) {
		t.mshrEntry = mshrEntries[s.MSHREntryIndex]
	}

	return t
}

func restoreTransMsgs(t *transaction, s transactionState) {
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
}

func restoreTransBlock(
	t *transaction,
	s transactionState,
	dir cache.Directory,
) {
	if s.HasBlock {
		sets := dir.GetSets()
		t.block = sets[s.BlockSetID].Blocks[s.BlockWayID]
	}
}

func restoreTransVictim(t *transaction, s transactionState) {
	if !s.HasVictim {
		return
	}

	v := &cache.Block{
		PID:          vm.PID(s.Victim.PID),
		Tag:          s.Victim.Tag,
		WayID:        s.Victim.WayID,
		SetID:        s.Victim.SetID,
		CacheAddress: s.Victim.CacheAddress,
		IsValid:      s.Victim.IsValid,
		IsDirty:      s.Victim.IsDirty,
		ReadCount:    s.Victim.ReadCount,
		IsLocked:     s.Victim.IsLocked,
	}

	if s.Victim.DirtyMask != nil {
		v.DirtyMask = make([]bool, len(s.Victim.DirtyMask))
		copy(v.DirtyMask, s.Victim.DirtyMask)
	}

	t.victim = v
}

func restoreTransData(t *transaction, s transactionState) {
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
}

func restoreTransFetchEvict(t *transaction, s transactionState) {
	t.fetchPID = vm.PID(s.FetchPID)
	t.fetchAddress = s.FetchAddress
	t.evictingPID = vm.PID(s.EvictingPID)
	t.evictingAddr = s.EvictingAddr

	if s.HasFetchReadReq {
		t.fetchReadReq = &mem.ReadReq{MsgMeta: s.FetchReadReqMsg}
	}

	if s.HasEvictionWriteReq {
		t.evictionWriteReq = &mem.WriteReq{
			MsgMeta: s.EvictionWriteReqMsg,
		}
	}
}

func snapshotDirBuf(
	buf queueing.Buffer,
	lookup map[*transaction]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		indices[i] = lookup[e.(*transaction)]
	}

	return indices
}

func restoreDirBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transaction,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = allTrans[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotBankBufs(
	bankBufs []queueing.Buffer,
	lookup map[*transaction]int,
) []bankBufState {
	result := make([]bankBufState, len(bankBufs))

	for i, buf := range bankBufs {
		elems := queueing.SnapshotBuffer(buf)
		indices := make([]int, len(elems))

		for j, e := range elems {
			indices[j] = lookup[e.(*transaction)]
		}

		result[i] = bankBufState{Indices: indices}
	}

	return result
}

func restoreBankBufs(
	bankBufs []queueing.Buffer,
	states []bankBufState,
	allTrans []*transaction,
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
	mshrLookup map[*cache.MSHREntry]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		entry := e.(*cache.MSHREntry)
		indices[i] = mshrLookup[entry]
	}

	return indices
}

func restoreMSHRStageBuf(
	buf queueing.Buffer,
	indices []int,
	mshrEntries []*cache.MSHREntry,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = mshrEntries[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotWriteBufferBuf(
	buf queueing.Buffer,
	lookup map[*transaction]int,
) []int {
	elems := queueing.SnapshotBuffer(buf)
	indices := make([]int, len(elems))

	for i, e := range elems {
		indices[i] = lookup[e.(*transaction)]
	}

	return indices
}

func restoreWriteBufferBuf(
	buf queueing.Buffer,
	indices []int,
	allTrans []*transaction,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = allTrans[idx]
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotDirPipeline(
	p queueing.Pipeline,
	lookup map[*transaction]int,
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
	allTrans []*transaction,
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
	lookup map[*transaction]int,
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
	allTrans []*transaction,
) {
	elems := make([]interface{}, len(indices))
	for i, idx := range indices {
		elems[i] = dirPipelineItem{trans: allTrans[idx]}
	}

	queueing.RestoreBuffer(buf, elems)
}

func snapshotBankPipelines(
	bankStages []*bankStage,
	lookup map[*transaction]int,
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
	allTrans []*transaction,
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
	lookup map[*transaction]int,
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
	allTrans []*transaction,
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

func snapshotTransList(
	list []*transaction,
	lookup map[*transaction]int,
) []int {
	indices := make([]int, len(list))
	for i, t := range list {
		indices[i] = lookup[t]
	}

	return indices
}

func restoreTransList(
	indices []int,
	allTrans []*transaction,
) []*transaction {
	list := make([]*transaction, len(indices))
	for i, idx := range indices {
		list[i] = allTrans[idx]
	}

	return list
}

func snapshotFlusherBlocks(
	blocks []*cache.Block,
) []blockRef {
	refs := make([]blockRef, len(blocks))
	for i, b := range blocks {
		refs[i] = blockRef{SetID: b.SetID, WayID: b.WayID}
	}

	return refs
}

func restoreFlusherBlocks(
	refs []blockRef,
	dir cache.Directory,
) []*cache.Block {
	sets := dir.GetSets()
	blocks := make([]*cache.Block, len(refs))

	for i, r := range refs {
		blocks[i] = sets[r.SetID].Blocks[r.WayID]
	}

	return blocks
}

func mshrTransLookup(
	lookup map[*transaction]int,
) map[interface{}]int {
	m := make(map[interface{}]int, len(lookup))
	for k, v := range lookup {
		m[k] = v
	}

	return m
}

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

func snapshotWriteBufferStage(
	wb *writeBufferStage,
	lookup map[*transaction]int,
) (pending, fetchInfl, evictInfl []int) {
	pending = snapshotTransList(wb.pendingEvictions, lookup)
	fetchInfl = snapshotTransList(wb.inflightFetch, lookup)
	evictInfl = snapshotTransList(wb.inflightEviction, lookup)

	return pending, fetchInfl, evictInfl
}

func snapshotMSHRStageEntry(
	ms *mshrStage,
	mshrLookup map[*cache.MSHREntry]int,
) (bool, int) {
	if ms.processingMSHREntry == nil {
		return false, 0
	}

	idx, ok := mshrLookup[ms.processingMSHREntry]
	if !ok {
		return true, 0
	}

	return true, idx
}

func snapshotFlusherState(
	f *flusher,
) ([]blockRef, bool, flushReqState) {
	refs := snapshotFlusherBlocks(f.blockToEvict)

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

func (c *Comp) snapshotState() State {
	lookup := buildTransIndex(c.inFlightTransactions)
	mshrLookup := buildMSHREntryLookup(c.mshr)

	s := State{
		CacheState:     int(c.state),
		DirectoryState: cache.SnapshotDirectory(c.directory),
		MSHRState: cache.SnapshotMSHR(
			c.mshr, mshrTransLookup(lookup)),
		Transactions: snapshotAllTransactions(
			c.inFlightTransactions, mshrLookup),
		EvictingList: snapshotEvictingList(c.evictingList),
	}

	s.DirStageBufIndices = snapshotDirBuf(c.dirStageBuffer, lookup)
	s.DirToBankBufIndices = snapshotBankBufs(c.dirToBankBuffers, lookup)
	s.WriteBufferToBankBufIndices = snapshotBankBufs(
		c.writeBufferToBankBuffers, lookup)
	s.MSHRStageBufEntries = snapshotMSHRStageBuf(
		c.mshrStageBuffer, mshrLookup)
	s.WriteBufferBufIndices = snapshotWriteBufferBuf(
		c.writeBufferBuffer, lookup)

	s.DirPipelineStages = snapshotDirPipeline(
		c.dirStage.pipeline, lookup)
	s.DirPostPipelineBufIndices = snapshotDirPostBuf(
		c.dirStage.buf, lookup)
	s.BankPipelineStages = snapshotBankPipelines(c.bankStages, lookup)
	s.BankPostPipelineBufIndices = snapshotBankPostBufs(
		c.bankStages, lookup)

	s.BankInflightTransCounts, s.BankDownwardInflightTransCounts =
		snapshotBankCounters(c.bankStages)
	s.PendingEvictionIndices, s.InflightFetchIndices,
		s.InflightEvictionIndices =
		snapshotWriteBufferStage(c.writeBuffer, lookup)
	s.HasProcessingMSHREntry, s.ProcessingMSHREntryIdx =
		snapshotMSHRStageEntry(c.mshrStage, mshrLookup)
	s.FlusherBlockToEvictRefs, s.HasProcessingFlush,
		s.ProcessingFlush = snapshotFlusherState(c.flusher)

	return s
}

func (c *Comp) restoreFromState(s State) {
	c.state = cacheState(s.CacheState)

	cache.RestoreDirectory(c.directory, s.DirectoryState)

	// Restore MSHR first to get entry pointers
	ifaces := make([]interface{}, 0)
	cache.RestoreMSHR(c.mshr, s.MSHRState, ifaces, c.directory)
	mshrEntries := c.mshr.AllEntries()

	// Restore transactions
	allTrans := restoreAllTransactions(
		s.Transactions, c.directory, mshrEntries)
	c.inFlightTransactions = allTrans

	// Re-wire MSHR entry Requests to point to restored transactions
	rewireMSHRRequests(c.mshr, s.MSHRState, allTrans)

	// Evicting list
	c.evictingList = make(map[uint64]bool)
	for k, v := range s.EvictingList {
		c.evictingList[k] = v
	}

	restoreBuffersAndPipelines(c, s, allTrans, mshrEntries)
	restoreBankCounters(c, s)
	restoreWriteBufferStage(c, s, allTrans)
	restoreMSHRStage(c, s, mshrEntries)
	restoreFlusherState(c, s)
}

func rewireMSHRRequests(
	m cache.MSHR,
	ms cache.MSHRState,
	allTrans []*transaction,
) {
	entries := m.AllEntries()
	for i, es := range ms.Entries {
		reqs := make([]interface{}, len(es.TransactionIndices))
		for j, idx := range es.TransactionIndices {
			reqs[j] = allTrans[idx]
		}

		entries[i].Requests = reqs
	}
}

func restoreBuffersAndPipelines(
	c *Comp,
	s State,
	allTrans []*transaction,
	mshrEntries []*cache.MSHREntry,
) {
	restoreDirBuf(c.dirStageBuffer, s.DirStageBufIndices, allTrans)
	restoreBankBufs(c.dirToBankBuffers,
		s.DirToBankBufIndices, allTrans)
	restoreBankBufs(c.writeBufferToBankBuffers,
		s.WriteBufferToBankBufIndices, allTrans)
	restoreMSHRStageBuf(c.mshrStageBuffer,
		s.MSHRStageBufEntries, mshrEntries)
	restoreWriteBufferBuf(c.writeBufferBuffer,
		s.WriteBufferBufIndices, allTrans)

	restoreDirPipeline(
		c.dirStage.pipeline, s.DirPipelineStages, allTrans)
	restoreDirPostBuf(
		c.dirStage.buf, s.DirPostPipelineBufIndices, allTrans)
	restoreBankPipelines(
		c.bankStages, s.BankPipelineStages, allTrans)
	restoreBankPostBufs(
		c.bankStages, s.BankPostPipelineBufIndices, allTrans)
}

func restoreBankCounters(c *Comp, s State) {
	for i, bs := range c.bankStages {
		if i < len(s.BankInflightTransCounts) {
			bs.inflightTransCount = s.BankInflightTransCounts[i]
		}

		if i < len(s.BankDownwardInflightTransCounts) {
			bs.downwardInflightTransCount =
				s.BankDownwardInflightTransCounts[i]
		}
	}
}

func restoreWriteBufferStage(
	c *Comp,
	s State,
	allTrans []*transaction,
) {
	c.writeBuffer.pendingEvictions = restoreTransList(
		s.PendingEvictionIndices, allTrans)
	c.writeBuffer.inflightFetch = restoreTransList(
		s.InflightFetchIndices, allTrans)
	c.writeBuffer.inflightEviction = restoreTransList(
		s.InflightEvictionIndices, allTrans)
}

func restoreMSHRStage(
	c *Comp,
	s State,
	mshrEntries []*cache.MSHREntry,
) {
	c.mshrStage.processingMSHREntry = nil

	if s.HasProcessingMSHREntry {
		idx := s.ProcessingMSHREntryIdx
		if idx >= 0 && idx < len(mshrEntries) {
			c.mshrStage.processingMSHREntry = mshrEntries[idx]
		}
	}
}

func restoreFlusherState(c *Comp, s State) {
	c.flusher.blockToEvict = restoreFlusherBlocks(
		s.FlusherBlockToEvictRefs, c.directory)
	c.flusher.processingFlush = nil

	if s.HasProcessingFlush {
		c.flusher.processingFlush = &cache.FlushReq{
			MsgMeta:                 s.ProcessingFlush.MsgMeta,
			InvalidateAllCachelines: s.ProcessingFlush.InvalidateAllCachelines,
			DiscardInflight:         s.ProcessingFlush.DiscardInflight,
			PauseAfterFlushing:      s.ProcessingFlush.PauseAfterFlushing,
		}
	}
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := c.snapshotState()
	c.Component.SetState(state)

	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)
	c.restoreFromState(state)
}

// SaveState marshals the component's spec and state as JSON, ensuring the
// runtime fields are synced into State first.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState reads JSON from r and restores both the base state and the
// runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}

	c.SetState(c.Component.GetState())

	return nil
}
