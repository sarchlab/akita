package dram

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/mem/dram/internal/cmdq"
	"github.com/sarchlab/akita/v5/mem/dram/internal/org"
	"github.com/sarchlab/akita/v5/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v5/mem/dram/internal/trans"
	"github.com/sarchlab/akita/v5/sim"
)

// msgRef is a serializable representation of a *sim.Msg.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// subTransRef identifies a SubTransaction by its parent transaction index
// and its position within that transaction's SubTransactions slice.
type subTransRef struct {
	TransIndex int `json:"trans_index"`
	SubIndex   int `json:"sub_index"`
}

// subTransState is a serializable representation of a SubTransaction.
type subTransState struct {
	ID               string `json:"id"`
	Address          uint64 `json:"address"`
	Completed        bool   `json:"completed"`
	TransactionIndex int    `json:"transaction_index"`
}

// transactionState is a serializable representation of a Transaction.
type transactionState struct {
	HasRead         bool            `json:"has_read"`
	HasWrite        bool            `json:"has_write"`
	ReadMsg         msgRef          `json:"read_msg"`
	WriteMsg        msgRef          `json:"write_msg"`
	InternalAddress uint64          `json:"internal_address"`
	SubTransactions []subTransState `json:"sub_transactions"`
}

// commandState is a serializable representation of a Command.
type commandState struct {
	ID        string `json:"id"`
	Kind      int    `json:"kind"`
	Address   uint64 `json:"address"`
	CycleLeft int    `json:"cycle_left"`
	Channel   uint64 `json:"channel"`
	Rank      uint64 `json:"rank"`
	BankGroup uint64 `json:"bank_group"`
	Bank      uint64 `json:"bank"`
	Row       uint64 `json:"row"`
	Column    uint64 `json:"column"`
	SubTransRef subTransRef `json:"sub_trans_ref"`
}

// bankEntry is a bankState tagged with its rank/bankGroup/bank indices.
type bankEntry struct {
	Rank      int        `json:"rank"`
	BankGroup int        `json:"bank_group"`
	BankIndex int        `json:"bank_index"`
	Data      bankState  `json:"data"`
}

// bankState is a serializable representation of a BankImpl.
type bankState struct {
	State                int              `json:"state"`
	OpenRow              uint64           `json:"open_row"`
	HasCurrentCmd        bool             `json:"has_current_cmd"`
	CurrentCmd           commandState     `json:"current_cmd"`
	CyclesToCmdAvailable map[string]int   `json:"cycles_to_cmd_available"`
}

// bankStatesFlat is a flattened representation of the 3D bank array.
type bankStatesFlat struct {
	NumRanks      int         `json:"num_ranks"`
	NumBankGroups int         `json:"num_bank_groups"`
	NumBanks      int         `json:"num_banks"`
	Entries       []bankEntry `json:"entries"`
}

// queueEntry is a command state tagged with its queue index.
type queueEntry struct {
	QueueIndex int          `json:"queue_index"`
	Command    commandState `json:"command"`
}

// commandQueueState is a serializable representation of CommandQueueImpl.
type commandQueueState struct {
	NumQueues      int          `json:"num_queues"`
	Entries        []queueEntry `json:"entries"`
	NextQueueIndex int          `json:"next_queue_index"`
}

// subTransQueueState is a list of sub-transaction references.
type subTransQueueState struct {
	Entries []subTransRef `json:"entries"`
}

func msgRefFromMsg(msg *sim.Msg) msgRef {
	return msgRef{
		ID:           msg.ID,
		Src:          msg.Src,
		Dst:          msg.Dst,
		RspTo:        msg.RspTo,
		TrafficClass: msg.TrafficClass,
		TrafficBytes: msg.TrafficBytes,
	}
}

func msgFromRef(ref msgRef) *sim.Msg {
	return &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:           ref.ID,
			Src:          ref.Src,
			Dst:          ref.Dst,
			TrafficClass: ref.TrafficClass,
			TrafficBytes: ref.TrafficBytes,
		},
		RspTo: ref.RspTo,
	}
}

// subTransLookup maps a *SubTransaction to its (transIndex, subIndex).
type subTransLookup map[*signal.SubTransaction]subTransRef

func buildSubTransLookup(
	transactions []*signal.Transaction,
) subTransLookup {
	lookup := make(subTransLookup)
	for ti, t := range transactions {
		for si, st := range t.SubTransactions {
			lookup[st] = subTransRef{
				TransIndex: ti,
				SubIndex:   si,
			}
		}
	}
	return lookup
}

func snapshotTransaction(
	t *signal.Transaction,
	transIndex int,
) transactionState {
	ts := transactionState{
		InternalAddress: t.InternalAddress,
	}

	if t.Read != nil {
		ts.HasRead = true
		ts.ReadMsg = msgRefFromMsg(t.Read)
	}

	if t.Write != nil {
		ts.HasWrite = true
		ts.WriteMsg = msgRefFromMsg(t.Write)
	}

	ts.SubTransactions = make([]subTransState, len(t.SubTransactions))
	for i, st := range t.SubTransactions {
		ts.SubTransactions[i] = subTransState{
			ID:               st.ID,
			Address:          st.Address,
			Completed:        st.Completed,
			TransactionIndex: transIndex,
		}
	}

	return ts
}

func snapshotCommand(
	cmd *signal.Command,
	lookup subTransLookup,
) commandState {
	cs := commandState{
		ID:        cmd.ID,
		Kind:      int(cmd.Kind),
		Address:   cmd.Address,
		CycleLeft: cmd.CycleLeft,
		Channel:   cmd.Location.Channel,
		Rank:      cmd.Location.Rank,
		BankGroup: cmd.Location.BankGroup,
		Bank:      cmd.Location.Bank,
		Row:       cmd.Location.Row,
		Column:    cmd.Location.Column,
	}

	if cmd.SubTrans != nil {
		cs.SubTransRef = lookup[cmd.SubTrans]
	}

	return cs
}

func snapshotBank(
	b *org.BankImpl,
	lookup subTransLookup,
) bankState {
	bs := bankState{
		State:   int(b.GetState()),
		OpenRow: b.GetOpenRow(),
		CyclesToCmdAvailable: snapshotCyclesToCmdAvailable(
			b.GetCyclesToCmdAvailable()),
	}

	currentCmd := b.GetCurrentCmd()
	if currentCmd != nil {
		bs.HasCurrentCmd = true
		bs.CurrentCmd = snapshotCommand(currentCmd, lookup)
	}

	return bs
}

func snapshotCyclesToCmdAvailable(
	m map[signal.CommandKind]int,
) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[fmt.Sprintf("%d", int(k))] = v
	}
	return out
}

func snapshotCommandQueue(
	cq *cmdq.CommandQueueImpl,
	lookup subTransLookup,
) commandQueueState {
	cqs := commandQueueState{
		NumQueues:      len(cq.Queues),
		NextQueueIndex: cq.GetNextQueueIndex(),
	}

	for i, q := range cq.Queues {
		for _, cmd := range q {
			cqs.Entries = append(cqs.Entries, queueEntry{
				QueueIndex: i,
				Command:    snapshotCommand(cmd, lookup),
			})
		}
	}

	if cqs.Entries == nil {
		cqs.Entries = []queueEntry{}
	}

	return cqs
}

func snapshotSubTransQueue(
	q *trans.FCFSSubTransactionQueue,
	lookup subTransLookup,
) subTransQueueState {
	sqs := subTransQueueState{
		Entries: make([]subTransRef, len(q.Queue)),
	}

	for i, st := range q.Queue {
		sqs.Entries[i] = lookup[st]
	}

	return sqs
}

func snapshotBanks(
	banks org.Banks,
	lookup subTransLookup,
) bankStatesFlat {
	flat := bankStatesFlat{
		NumRanks:      len(banks),
		NumBankGroups: len(banks[0]),
		NumBanks:      len(banks[0][0]),
	}

	for i := range banks {
		for j := range banks[i] {
			for k := range banks[i][j] {
				bi := banks[i][j][k].(*org.BankImpl)
				flat.Entries = append(flat.Entries,
					bankEntry{
						Rank:      i,
						BankGroup: j,
						BankIndex: k,
						Data:      snapshotBank(bi, lookup),
					})
			}
		}
	}

	if flat.Entries == nil {
		flat.Entries = []bankEntry{}
	}

	return flat
}

// snapshotState converts runtime mutable data into a serializable State.
func (c *Comp) snapshotState() State {
	lookup := buildSubTransLookup(c.inflightTransactions)

	s := State{}
	s.Transactions = snapshotTransactions(c.inflightTransactions)

	stq := c.subTransactionQueue.(*trans.FCFSSubTransactionQueue)
	s.SubTransQueue = snapshotSubTransQueue(stq, lookup)

	cqi := c.cmdQueue.(*cmdq.CommandQueueImpl)
	s.CommandQueues = snapshotCommandQueue(cqi, lookup)

	chi := c.channel.(*org.ChannelImpl)
	s.BankStates = snapshotBanks(chi.Banks, lookup)

	return s
}

func snapshotTransactions(
	transactions []*signal.Transaction,
) []transactionState {
	states := make([]transactionState, len(transactions))
	for i, t := range transactions {
		states[i] = snapshotTransaction(t, i)
	}
	return states
}

// restoreFromState restores runtime mutable data from a serializable State.
func (c *Comp) restoreFromState(s State) {
	transactions := restoreTransactions(s.Transactions)
	c.inflightTransactions = transactions

	stq := c.subTransactionQueue.(*trans.FCFSSubTransactionQueue)
	restoreSubTransQueue(stq, s.SubTransQueue, transactions)

	cqi := c.cmdQueue.(*cmdq.CommandQueueImpl)
	restoreCommandQueue(cqi, s.CommandQueues, transactions)

	chi := c.channel.(*org.ChannelImpl)
	restoreBanks(chi.Banks, s.BankStates, transactions)
}

func restoreTransactions(
	states []transactionState,
) []*signal.Transaction {
	transactions := make([]*signal.Transaction, len(states))
	for i, ts := range states {
		transactions[i] = restoreTransaction(ts)
	}

	// Wire back-pointers.
	for _, t := range transactions {
		for _, st := range t.SubTransactions {
			st.Transaction = t
		}
	}

	return transactions
}

func restoreTransaction(ts transactionState) *signal.Transaction {
	t := &signal.Transaction{
		InternalAddress: ts.InternalAddress,
	}

	if ts.HasRead {
		t.Read = msgFromRef(ts.ReadMsg)
	}

	if ts.HasWrite {
		t.Write = msgFromRef(ts.WriteMsg)
	}

	t.SubTransactions = make(
		[]*signal.SubTransaction, len(ts.SubTransactions))
	for i, ss := range ts.SubTransactions {
		t.SubTransactions[i] = &signal.SubTransaction{
			ID:        ss.ID,
			Address:   ss.Address,
			Completed: ss.Completed,
		}
	}

	return t
}

func resolveSubTrans(
	ref subTransRef,
	transactions []*signal.Transaction,
) *signal.SubTransaction {
	return transactions[ref.TransIndex].
		SubTransactions[ref.SubIndex]
}

func restoreSubTransQueue(
	q *trans.FCFSSubTransactionQueue,
	sqs subTransQueueState,
	transactions []*signal.Transaction,
) {
	q.Queue = make([]*signal.SubTransaction, len(sqs.Entries))
	for i, ref := range sqs.Entries {
		q.Queue[i] = resolveSubTrans(ref, transactions)
	}
}

func restoreCommand(
	cs commandState,
	transactions []*signal.Transaction,
) *signal.Command {
	cmd := &signal.Command{
		ID:        cs.ID,
		Kind:      signal.CommandKind(cs.Kind),
		Address:   cs.Address,
		CycleLeft: cs.CycleLeft,
	}

	cmd.Location.Channel = cs.Channel
	cmd.Location.Rank = cs.Rank
	cmd.Location.BankGroup = cs.BankGroup
	cmd.Location.Bank = cs.Bank
	cmd.Location.Row = cs.Row
	cmd.Location.Column = cs.Column
	cmd.SubTrans = resolveSubTrans(
		cs.SubTransRef, transactions)

	return cmd
}

func restoreCommandQueue(
	cq *cmdq.CommandQueueImpl,
	cqs commandQueueState,
	transactions []*signal.Transaction,
) {
	cq.SetNextQueueIndex(cqs.NextQueueIndex)

	for i := range cq.Queues {
		cq.Queues[i] = nil
	}

	for _, entry := range cqs.Entries {
		cmd := restoreCommand(entry.Command, transactions)
		cq.Queues[entry.QueueIndex] = append(
			cq.Queues[entry.QueueIndex], cmd)
	}
}

func restoreBanks(
	banks org.Banks,
	flat bankStatesFlat,
	transactions []*signal.Transaction,
) {
	for _, entry := range flat.Entries {
		bi := banks[entry.Rank][entry.BankGroup][entry.BankIndex].(*org.BankImpl)
		restoreBank(bi, entry.Data, transactions)
	}
}

func restoreBank(
	b *org.BankImpl,
	bs bankState,
	transactions []*signal.Transaction,
) {
	b.SetState(org.BankState(bs.State))
	b.SetOpenRow(bs.OpenRow)
	restoreBankCyclesToCmd(b, bs.CyclesToCmdAvailable)

	if bs.HasCurrentCmd {
		cmd := restoreCommand(bs.CurrentCmd, transactions)
		b.SetCurrentCmd(cmd)
	} else {
		b.SetCurrentCmd(nil)
	}
}

func restoreBankCyclesToCmd(
	b *org.BankImpl,
	m map[string]int,
) {
	cycles := make(map[signal.CommandKind]int, len(m))

	for k, v := range m {
		var kindInt int
		_, _ = fmt.Sscanf(k, "%d", &kindInt)
		cycles[signal.CommandKind(kindInt)] = v
	}

	b.SetCyclesToCmdAvailable(cycles)
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
