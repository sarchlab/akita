package writearound

import (
	"github.com/sarchlab/akita/v5/mem/mem"
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
	BottomWriteDone       bool        `json:"bottom_write_done"`
	BankDone              bool        `json:"bank_done"`
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
		FetchAndWrite:   t.fetchAndWrite,
		Done:            t.done,
		BottomWriteDone: t.bottomWriteDone,
		BankDone:        t.bankDone,
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
	if t.hasBlock {
		s.HasBlock = true
		s.BlockSetID = t.blockSetID
		s.BlockWayID = t.blockWayID
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
) ([]*transactionState, []*transactionState) {
	allTrans := make([]*transactionState, len(snapshots))

	for i, s := range snapshots {
		allTrans[i] = restoreTransactionCore(s)
	}

	wirePreCoalesce(allTrans, snapshots)

	return allTrans[:numTrans], allTrans[numTrans:]
}

func restoreTransactionCore(
	s transactionSnapshot,
) *transactionState {
	t := &transactionState{
		id:            s.ID,
		bankAction:    bankActionType(s.BankAction),
		fetchAndWrite:   s.FetchAndWrite,
		done:            s.Done,
		bottomWriteDone: s.BottomWriteDone,
		bankDone:        s.BankDone,
	}

	restoreTransMsgs(t, s)
	restoreTransBlock(t, s)
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
) {
	if s.HasBlock {
		t.hasBlock = true
		t.blockSetID = s.BlockSetID
		t.blockWayID = s.BlockWayID
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
