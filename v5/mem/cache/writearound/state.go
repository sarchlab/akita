package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// transactionSnapshot is the serializable representation of a transactionState.
type transactionSnapshot struct {
	ID                    string      `json:"id"`
	HasRead               bool        `json:"has_read"`
	ReadMsg               sim.MsgMeta `json:"read_msg"`
	HasReadToBottom       bool        `json:"has_read_to_bottom"`
	ReadToBottomMsg       sim.MsgMeta `json:"read_to_bottom_msg"`
	HasWrite              bool        `json:"has_write"`
	WriteMsg              sim.MsgMeta `json:"write_msg"`
	HasWriteToBottom      bool        `json:"has_write_to_bottom"`
	WriteToBottomMsg      sim.MsgMeta `json:"write_to_bottom_msg"`
	PreCoalesceTransIdxs  []int       `json:"pre_coalesce_trans_idxs"`
	BankAction            int         `json:"bank_action"`
	HasBlock              bool        `json:"has_block"`
	BlockSetID            int         `json:"block_set_id"`
	BlockWayID            int         `json:"block_way_id"`
	Data                  []uint8     `json:"data"`
	WriteFetchedDirtyMask []bool      `json:"write_fetched_dirty_mask"`
	FetchAndWrite         bool        `json:"fetch_and_write"`
	Done                  bool        `json:"done"`
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

func buildTransIndex(
	transactions []*transactionState,
	postCoalesceTransactions []*transactionState,
) map[*transactionState]int {
	m := make(map[*transactionState]int,
		len(transactions)+len(postCoalesceTransactions))

	for i, t := range transactions {
		m[t] = i
	}

	base := len(transactions)

	for i, t := range postCoalesceTransactions {
		m[t] = base + i
	}

	return m
}

func snapshotTransaction(
	t *transactionState,
	lookup map[*transactionState]int,
) transactionSnapshot {
	s := transactionSnapshot{
		ID:            t.id,
		BankAction:    int(t.bankAction),
		FetchAndWrite: t.fetchAndWrite,
		Done:          t.done,
	}

	if t.read != nil {
		s.HasRead = true
		s.ReadMsg = t.read.MsgMeta
	}

	if t.readToBottom != nil {
		s.HasReadToBottom = true
		s.ReadToBottomMsg = t.readToBottom.MsgMeta
	}

	if t.write != nil {
		s.HasWrite = true
		s.WriteMsg = t.write.MsgMeta
	}

	if t.writeToBottom != nil {
		s.HasWriteToBottom = true
		s.WriteToBottomMsg = t.writeToBottom.MsgMeta
	}

	snapshotTransBlock(t, &s)
	snapshotTransData(t, &s)
	snapshotPreCoalesce(t, &s, lookup)

	return s
}

func snapshotTransBlock(t *transactionState, s *transactionSnapshot) {
	if t.block != nil {
		s.HasBlock = true
		s.BlockSetID = t.block.SetID
		s.BlockWayID = t.block.WayID
	}
}

func snapshotTransData(t *transactionState, s *transactionSnapshot) {
	if t.data != nil {
		s.Data = make([]uint8, len(t.data))
		copy(s.Data, t.data)
	}

	if t.writeFetchedDirtyMask != nil {
		s.WriteFetchedDirtyMask = make(
			[]bool, len(t.writeFetchedDirtyMask))
		copy(s.WriteFetchedDirtyMask, t.writeFetchedDirtyMask)
	}
}

func snapshotPreCoalesce(
	t *transactionState,
	s *transactionSnapshot,
	lookup map[*transactionState]int,
) {
	if len(t.preCoalesceTransactions) > 0 {
		s.PreCoalesceTransIdxs = make(
			[]int, len(t.preCoalesceTransactions))
		for j, pt := range t.preCoalesceTransactions {
			s.PreCoalesceTransIdxs[j] = lookup[pt]
		}
	}
}

func snapshotAllTransactions(
	transactions []*transactionState,
	postCoalesce []*transactionState,
	lookup map[*transactionState]int,
) []transactionSnapshot {
	total := len(transactions) + len(postCoalesce)
	states := make([]transactionSnapshot, total)

	for i, t := range transactions {
		states[i] = snapshotTransaction(t, lookup)
	}

	base := len(transactions)

	for i, t := range postCoalesce {
		states[base+i] = snapshotTransaction(t, lookup)
	}

	return states
}

func restoreAllTransactions(
	snapshots []transactionSnapshot,
	numTrans int,
	dir cache.Directory,
) ([]*transactionState, []*transactionState) {
	allTrans := make([]*transactionState, len(snapshots))

	for i, s := range snapshots {
		allTrans[i] = restoreTransactionCore(s, dir)
	}

	wirePreCoalesce(allTrans, snapshots)

	return allTrans[:numTrans], allTrans[numTrans:]
}

func restoreTransactionCore(
	s transactionSnapshot,
	dir cache.Directory,
) *transactionState {
	t := &transactionState{
		id:            s.ID,
		bankAction:    bankActionType(s.BankAction),
		fetchAndWrite: s.FetchAndWrite,
		done:          s.Done,
	}

	restoreTransMsgs(t, s)
	restoreTransBlock(t, s, dir)
	restoreTransData(t, s)

	return t
}

func restoreTransMsgs(t *transactionState, s transactionSnapshot) {
	if s.HasRead {
		t.read = &mem.ReadReq{MsgMeta: s.ReadMsg}
	}

	if s.HasReadToBottom {
		t.readToBottom = &mem.ReadReq{MsgMeta: s.ReadToBottomMsg}
	}

	if s.HasWrite {
		t.write = &mem.WriteReq{MsgMeta: s.WriteMsg}
	}

	if s.HasWriteToBottom {
		t.writeToBottom = &mem.WriteReq{MsgMeta: s.WriteToBottomMsg}
	}
}

func restoreTransBlock(
	t *transactionState,
	s transactionSnapshot,
	dir cache.Directory,
) {
	if s.HasBlock {
		sets := dir.GetSets()
		t.block = sets[s.BlockSetID].Blocks[s.BlockWayID]
	}
}

func restoreTransData(t *transactionState, s transactionSnapshot) {
	if s.Data != nil {
		t.data = make([]byte, len(s.Data))
		copy(t.data, s.Data)
	}

	if s.WriteFetchedDirtyMask != nil {
		t.writeFetchedDirtyMask = make(
			[]bool, len(s.WriteFetchedDirtyMask))
		copy(t.writeFetchedDirtyMask, s.WriteFetchedDirtyMask)
	}
}

func wirePreCoalesce(
	allTrans []*transactionState,
	snapshots []transactionSnapshot,
) {
	for i, s := range snapshots {
		if len(s.PreCoalesceTransIdxs) == 0 {
			continue
		}

		refs := make([]*transactionState, len(s.PreCoalesceTransIdxs))
		for j, idx := range s.PreCoalesceTransIdxs {
			refs[j] = allTrans[idx]
		}

		allTrans[i].preCoalesceTransactions = refs
	}
}

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
			bt := s.Elem.(*bankTransaction)
			states[j] = bankPipelineStageState{
				Lane:       s.Lane,
				Stage:      s.Stage,
				TransIndex: lookup[bt.transactionState],
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
				Elem: &bankTransaction{
					transactionState: allTrans[s.TransIndex],
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
			bt := e.(*bankTransaction)
			indices[j] = lookup[bt.transactionState]
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
			elems[j] = &bankTransaction{
				transactionState: allTrans[idx],
			}
		}

		queueing.RestoreBuffer(
			bankStages[i].postPipelineBuf, elems)
	}
}

func mshrTransLookup(
	lookup map[*transactionState]int,
) map[interface{}]int {
	m := make(map[interface{}]int, len(lookup))
	for k, v := range lookup {
		m[k] = v
	}

	return m
}
