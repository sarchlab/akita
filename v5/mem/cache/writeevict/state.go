package writeevict

import (
	"io"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// transactionState is the serializable representation of a transaction.
type transactionState struct {
	ID                     string       `json:"id"`
	HasRead                bool         `json:"has_read"`
	ReadMsg                sim.MsgMeta  `json:"read_msg"`
	HasReadToBottom        bool         `json:"has_read_to_bottom"`
	ReadToBottomMsg        sim.MsgMeta  `json:"read_to_bottom_msg"`
	HasWrite               bool         `json:"has_write"`
	WriteMsg               sim.MsgMeta  `json:"write_msg"`
	HasWriteToBottom       bool         `json:"has_write_to_bottom"`
	WriteToBottomMsg       sim.MsgMeta  `json:"write_to_bottom_msg"`
	PreCoalesceTransIdxs   []int        `json:"pre_coalesce_trans_idxs"`
	BankAction             int          `json:"bank_action"`
	HasBlock               bool         `json:"has_block"`
	BlockSetID             int          `json:"block_set_id"`
	BlockWayID             int          `json:"block_way_id"`
	Data                   []uint8      `json:"data"`
	WriteFetchedDirtyMask  []bool       `json:"write_fetched_dirty_mask"`
	FetchAndWrite          bool         `json:"fetch_and_write"`
	Done                   bool         `json:"done"`
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
	transactions []*transaction,
	postCoalesceTransactions []*transaction,
) map[*transaction]int {
	m := make(map[*transaction]int,
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
	t *transaction,
	lookup map[*transaction]int,
) transactionState {
	s := transactionState{
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

func snapshotTransBlock(t *transaction, s *transactionState) {
	if t.block != nil {
		s.HasBlock = true
		s.BlockSetID = t.block.SetID
		s.BlockWayID = t.block.WayID
	}
}

func snapshotTransData(t *transaction, s *transactionState) {
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
	t *transaction,
	s *transactionState,
	lookup map[*transaction]int,
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
	transactions []*transaction,
	postCoalesce []*transaction,
	lookup map[*transaction]int,
) []transactionState {
	total := len(transactions) + len(postCoalesce)
	states := make([]transactionState, total)

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
	states []transactionState,
	numTrans int,
	dir cache.Directory,
) ([]*transaction, []*transaction) {
	allTrans := make([]*transaction, len(states))

	for i, s := range states {
		allTrans[i] = restoreTransactionCore(s, dir)
	}

	wirePreCoalesce(allTrans, states)

	return allTrans[:numTrans], allTrans[numTrans:]
}

func restoreTransactionCore(
	s transactionState,
	dir cache.Directory,
) *transaction {
	t := &transaction{
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

func restoreTransMsgs(t *transaction, s transactionState) {
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
	t *transaction,
	s transactionState,
	dir cache.Directory,
) {
	if s.HasBlock {
		sets := dir.GetSets()
		t.block = sets[s.BlockSetID].Blocks[s.BlockWayID]
	}
}

func restoreTransData(t *transaction, s transactionState) {
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
	allTrans []*transaction,
	states []transactionState,
) {
	for i, s := range states {
		if len(s.PreCoalesceTransIdxs) == 0 {
			continue
		}

		refs := make([]*transaction, len(s.PreCoalesceTransIdxs))
		for j, idx := range s.PreCoalesceTransIdxs {
			refs[j] = allTrans[idx]
		}

		allTrans[i].preCoalesceTransactions = refs
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
			bt := s.Elem.(*bankTransaction)
			states[j] = bankPipelineStageState{
				Lane:       s.Lane,
				Stage:      s.Stage,
				TransIndex: lookup[bt.transaction],
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
				Elem: &bankTransaction{
					transaction: allTrans[s.TransIndex],
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
			bt := e.(*bankTransaction)
			indices[j] = lookup[bt.transaction]
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
			elems[j] = &bankTransaction{
				transaction: allTrans[idx],
			}
		}

		queueing.RestoreBuffer(
			bankStages[i].postPipelineBuf, elems)
	}
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

func (c *Comp) snapshotState() State {
	lookup := buildTransIndex(
		c.transactions, c.postCoalesceTransactions)

	s := State{
		IsPaused:       c.isPaused,
		NumTransactions: len(c.transactions),
	}

	s.DirectoryState = cache.SnapshotDirectory(c.directory)
	s.MSHRState = cache.SnapshotMSHR(
		c.mshr, mshrTransLookup(lookup))
	s.Transactions = snapshotAllTransactions(
		c.transactions, c.postCoalesceTransactions, lookup)
	s.DirBufIndices = snapshotDirBuf(c.dirBuf, lookup)
	s.BankBufIndices = snapshotBankBufs(c.bankBufs, lookup)
	s.DirPipelineStages = snapshotDirPipeline(
		c.directoryStage.pipeline, lookup)
	s.DirPostPipelineBufIndices = snapshotDirPostBuf(
		c.directoryStage.buf, lookup)
	s.BankPipelineStages = snapshotBankPipelines(
		c.bankStages, lookup)
	s.BankPostPipelineBufIndices = snapshotBankPostBufs(
		c.bankStages, lookup)

	return s
}

func (c *Comp) restoreFromState(s State) {
	c.isPaused = s.IsPaused

	cache.RestoreDirectory(c.directory, s.DirectoryState)

	trans, postCoalesce := restoreAllTransactions(
		s.Transactions, s.NumTransactions, c.directory)
	c.transactions = trans
	c.postCoalesceTransactions = postCoalesce

	allTrans := make([]*transaction, len(s.Transactions))
	copy(allTrans[:s.NumTransactions], trans)
	copy(allTrans[s.NumTransactions:], postCoalesce)

	restoreMSHR(c, s, allTrans)
	restoreBuffersAndPipelines(c, s, allTrans)
}

func restoreMSHR(
	c *Comp,
	s State,
	allTrans []*transaction,
) {
	ifaces := make([]interface{}, len(allTrans))
	for i, t := range allTrans {
		ifaces[i] = t
	}

	cache.RestoreMSHR(c.mshr, s.MSHRState, ifaces, c.directory)
}

func restoreBuffersAndPipelines(
	c *Comp,
	s State,
	allTrans []*transaction,
) {
	restoreDirBuf(c.dirBuf, s.DirBufIndices, allTrans)
	restoreBankBufs(c.bankBufs, s.BankBufIndices, allTrans)
	restoreDirPipeline(
		c.directoryStage.pipeline, s.DirPipelineStages, allTrans)
	restoreDirPostBuf(
		c.directoryStage.buf, s.DirPostPipelineBufIndices, allTrans)
	restoreBankPipelines(c.bankStages, s.BankPipelineStages, allTrans)
	restoreBankPostBufs(
		c.bankStages, s.BankPostPipelineBufIndices, allTrans)
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
