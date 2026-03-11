package dram

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Protocol defines the category of the memory controller.
type Protocol int

// A list of all supported DRAM protocols.
const (
	DDR3 Protocol = iota
	DDR4
	GDDR5
	GDDR5X
	GDDR6
	LPDDR
	LPDDR3
	LPDDR4
	HBM
	HBM2
	HMC
)

func (p Protocol) isGDDR() bool {
	return p == GDDR5 || p == GDDR5X || p == GDDR6
}

func (p Protocol) isHBM() bool {
	return p == HBM || p == HBM2
}

// Spec contains immutable configuration for the DRAM memory controller.
type Spec struct {
	// Protocol
	Protocol int `json:"protocol"`

	// Timing params
	TAL        int `json:"t_al"`
	TCL        int `json:"t_cl"`
	TCWL       int `json:"t_cwl"`
	TRL        int `json:"t_rl"`
	TWL        int `json:"t_wl"`
	ReadDelay  int `json:"read_delay"`
	WriteDelay int `json:"write_delay"`
	TRCD       int `json:"t_rcd"`
	TRP        int `json:"t_rp"`
	TRAS       int `json:"t_ras"`
	TCCDS      int `json:"t_ccds"`
	TCCDL      int `json:"t_ccdl"`
	TRTRS      int `json:"t_rtrs"`
	TRTP       int `json:"t_rtp"`
	TWTRL      int `json:"t_wtrl"`
	TWTRS      int `json:"t_wtrs"`
	TWR        int `json:"t_wr"`
	TPPD       int `json:"t_ppd"`
	TRC        int `json:"t_rc"`
	TRRDS      int `json:"t_rrds"`
	TRRDL      int `json:"t_rrdl"`
	TRCDRD     int `json:"t_rcdrd"`
	TRCDWR     int `json:"t_rcdwr"`
	TREFI      int `json:"t_refi"`
	TRFC       int `json:"t_rfc"`
	TRFCb      int `json:"t_rfcb"`
	TCKESR     int `json:"t_ckesr"`
	TXS        int `json:"t_xs"`
	BurstCycle int `json:"burst_cycle"`

	// Bus / burst / device params
	BusWidth    int `json:"bus_width"`
	BurstLength int `json:"burst_length"`
	DeviceWidth int `json:"device_width"`

	// Bank / rank / channel counts
	NumChannel  int `json:"num_channel"`
	NumRank     int `json:"num_rank"`
	NumBankGroup int `json:"num_bank_group"`
	NumBank     int `json:"num_bank"`
	NumRow      int `json:"num_row"`
	NumCol      int `json:"num_col"`

	// Queue sizes
	TransactionQueueSize int `json:"transaction_queue_size"`
	CommandQueueCapacity int `json:"command_queue_capacity"`

	// Address converter params
	HasAddrConverter    bool   `json:"has_addr_converter"`
	InterleavingSize    uint64 `json:"interleaving_size"`
	TotalNumOfElements  int    `json:"total_num_of_elements"`
	CurrentElementIndex int    `json:"current_element_index"`
	Offset              uint64 `json:"offset"`

	// Address mapping: position/mask pairs
	ChannelPos    int    `json:"channel_pos"`
	ChannelMask   uint64 `json:"channel_mask"`
	RankPos       int    `json:"rank_pos"`
	RankMask      uint64 `json:"rank_mask"`
	BankGroupPos  int    `json:"bank_group_pos"`
	BankGroupMask uint64 `json:"bank_group_mask"`
	BankPos       int    `json:"bank_pos"`
	BankMask      uint64 `json:"bank_mask"`
	RowPos        int    `json:"row_pos"`
	RowMask       uint64 `json:"row_mask"`
	ColPos        int    `json:"col_pos"`
	ColMask       uint64 `json:"col_mask"`

	// Sub-transaction splitting
	Log2AccessUnitSize uint64 `json:"log2_access_unit_size"`

	// CmdCycles: cycles per command kind
	CmdCycles map[CommandKind]int `json:"cmd_cycles"`

	// The entire Timing structure (computed once in builder)
	Timing Timing `json:"timing"`
}

// State contains mutable runtime data for the DRAM memory controller.
type State struct {
	Transactions  []transactionState `json:"transactions"`
	SubTransQueue subTransQueueState `json:"sub_trans_queue"`
	CommandQueues commandQueueState  `json:"command_queues"`
	BankStates    bankStatesFlat     `json:"bank_states"`
}

// Comp is a MemController that handles read and write requests.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort sim.Port
	storage *mem.Storage
}

type middleware struct {
	*Comp
}

// Tick updates memory controller's internal state.
func (m *middleware) Tick() (madeProgress bool) {
	state := m.Comp.GetNextState()
	spec := m.Comp.GetSpec()

	progress := false

	progress = m.respond(&spec, state) || progress
	progress = m.respond(&spec, state) || progress
	progress = tickBanks(&spec, state) || progress
	progress = m.issue(&spec, state) || progress
	progress = tickSubTransQueue(&spec, state) || progress
	progress = m.parseTop(&spec, state) || progress

	return progress
}

func (m *middleware) parseTop(spec *Spec, state *State) bool {
	msgI := m.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	ts := transactionState{}

	switch msg := msgI.(type) {
	case *mem.ReadReq:
		ts.HasRead = true
		ts.ReadMsg = *msg
	case *mem.WriteReq:
		ts.HasWrite = true
		ts.WriteMsg = *msg
	}

	// Assign internal address
	globalAddr := transactionGlobalAddress(&ts)
	ts.InternalAddress = convertExternalToInternal(spec, globalAddr)

	// Split into sub-transactions
	transIdx := len(state.Transactions)
	splitTransaction(spec, &ts, transIdx)

	if !canPushSubTrans(state, len(ts.SubTransactions),
		spec.TransactionQueueSize) {
		return false
	}

	state.Transactions = append(state.Transactions, ts)
	pushSubTrans(state, transIdx)
	m.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msgI, m.Comp)

	for _, st := range ts.SubTransactions {
		tracing.StartTaskWithSpecificLocation(
			st.ID,
			tracing.MsgIDAtReceiver(msgI, m.Comp),
			m.Comp,
			"sub-trans",
			"sub-trans",
			m.Comp.Name()+".SubTransQueue",
			nil,
		)
	}

	return true
}

func (m *middleware) issue(spec *Spec, state *State) bool {
	cmd := getCommandToIssue(spec, state)
	if cmd == nil {
		return false
	}

	bs := findBankStateByLocation(&state.BankStates, cmd.Location)
	if bs == nil {
		return false
	}

	startCommand(spec, state, bs, cmd)
	updateTiming(spec, state, cmd)

	return true
}

func (m *middleware) respond(spec *Spec, state *State) bool {
	for i := range state.Transactions {
		t := &state.Transactions[i]
		if isTransactionCompleted(t) {
			done := m.finalizeTransaction(spec, state, t, i)
			if done {
				return true
			}
		}
	}

	return false
}

func (m *middleware) finalizeTransaction(
	spec *Spec,
	state *State,
	t *transactionState,
	i int,
) bool {
	if t.HasWrite {
		done := m.finalizeWriteTrans(state, t, i)
		if done {
			tracing.TraceReqComplete(&t.WriteMsg, m.Comp)
		}
		return done
	}

	done := m.finalizeReadTrans(state, t, i)
	if done {
		tracing.TraceReqComplete(&t.ReadMsg, m.Comp)
	}
	return done
}

func (m *middleware) finalizeWriteTrans(
	state *State,
	t *transactionState,
	i int,
) bool {
	err := m.storage.Write(t.InternalAddress, t.WriteMsg.Data)
	if err != nil {
		panic(err)
	}

	writeDone := &mem.WriteDoneRsp{}
	writeDone.ID = sim.GetIDGenerator().Generate()
	writeDone.Src = m.topPort.AsRemote()
	writeDone.Dst = t.WriteMsg.Src
	writeDone.RspTo = t.WriteMsg.ID
	writeDone.TrafficBytes = 4
	writeDone.TrafficClass = "mem.WriteDoneRsp"

	sendErr := m.topPort.Send(writeDone)
	if sendErr == nil {
		m.removeTransaction(state, i)
		return true
	}

	return false
}

func (m *middleware) finalizeReadTrans(
	state *State,
	t *transactionState,
	i int,
) bool {
	data, err := m.storage.Read(
		t.InternalAddress, t.ReadMsg.AccessByteSize)
	if err != nil {
		panic(err)
	}

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = m.topPort.AsRemote()
	dataReady.Dst = t.ReadMsg.Src
	dataReady.Data = data
	dataReady.RspTo = t.ReadMsg.ID
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"

	sendErr := m.topPort.Send(dataReady)
	if sendErr == nil {
		m.removeTransaction(state, i)
		return true
	}

	return false
}

// removeTransaction removes a transaction and remaps all references.
func (m *middleware) removeTransaction(state *State, idx int) {
	// Remove the transaction
	state.Transactions = append(
		state.Transactions[:idx],
		state.Transactions[idx+1:]...,
	)

	// Remap sub-trans queue references
	newEntries := state.SubTransQueue.Entries[:0]
	for _, ref := range state.SubTransQueue.Entries {
		if ref.TransIndex == idx {
			continue // remove refs to deleted transaction
		}
		if ref.TransIndex > idx {
			ref.TransIndex--
		}
		newEntries = append(newEntries, ref)
	}
	state.SubTransQueue.Entries = newEntries

	// Remap command queue references
	newCmdEntries := state.CommandQueues.Entries[:0]
	for _, e := range state.CommandQueues.Entries {
		if e.Command.SubTransRef.TransIndex == idx {
			continue
		}
		if e.Command.SubTransRef.TransIndex > idx {
			e.Command.SubTransRef.TransIndex--
		}
		newCmdEntries = append(newCmdEntries, e)
	}
	state.CommandQueues.Entries = newCmdEntries

	// Remap bank current command references
	for i := range state.BankStates.Entries {
		bs := &state.BankStates.Entries[i].Data
		if bs.HasCurrentCmd {
			if bs.CurrentCmd.SubTransRef.TransIndex == idx {
				// The transaction is done, so this cmd should already
				// be completed. But just in case, update.
				bs.CurrentCmd.SubTransRef.TransIndex = -1
			} else if bs.CurrentCmd.SubTransRef.TransIndex > idx {
				bs.CurrentCmd.SubTransRef.TransIndex--
			}
		}
	}

	// Also update TransactionIndex in SubTransactions
	for ti := range state.Transactions {
		for si := range state.Transactions[ti].SubTransactions {
			state.Transactions[ti].SubTransactions[si].TransactionIndex = ti
		}
	}
}
