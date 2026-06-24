package dram

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// protocol defines the category of the memory controller.
type protocol int

// A list of all supported DRAM protocols.
const (
	protoDDR3 protocol = iota
	protoDDR4
	protoGDDR5
	protoGDDR5X
	protoGDDR6
	protoLPDDR
	protoLPDDR3
	protoLPDDR4
	protoHBM
	protoHBM2
	protoHMC
	protoDDR5
	protoHBM3
	protoLPDDR5
	protoHBM3E
)

func (p protocol) isGDDR() bool {
	return p == protoGDDR5 || p == protoGDDR5X || p == protoGDDR6
}

func (p protocol) isHBM() bool {
	return p == protoHBM || p == protoHBM2 || p == protoHBM3 || p == protoHBM3E
}

// PagePolicy defines the page management policy for the DRAM controller.
type PagePolicy int

// A list of supported page policies.
const (
	PagePolicyClose PagePolicy = 0
	PagePolicyOpen  PagePolicy = 1
)

// Spec contains immutable configuration for the DRAM memory controller.
type Spec struct {
	// Frequency
	Freq timing.Freq `json:"freq"`

	// Protocol
	Protocol int `json:"protocol"`

	// Page policy
	PagePolicy PagePolicy `json:"page_policy"`

	// Strategy selection (registry keys; "" selects the default). The row
	// policy is selected from PagePolicy. See plugins.go.
	Scheduler  string `json:"scheduler"`
	AddrMapper string `json:"addr_mapper"`

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
	TFAW       int `json:"t_faw"`
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
	NumChannel   int `json:"num_channel"`
	NumRank      int `json:"num_rank"`
	NumBankGroup int `json:"num_bank_group"`
	NumBank      int `json:"num_bank"`
	NumRow       int `json:"num_row"`
	NumCol       int `json:"num_col"`

	// Queue sizes
	TransactionQueueSize int `json:"transaction_queue_size"`
	CommandQueueCapacity int `json:"command_queue_capacity"`

	// Read/Write queue separation
	ReadQueueSize      int `json:"read_queue_size"`
	WriteQueueSize     int `json:"write_queue_size"`
	WriteHighWatermark int `json:"write_high_watermark"`
	WriteLowWatermark  int `json:"write_low_watermark"`

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
}

// commandKind represents the kind of the command.
type commandKind int

// A list of supported DRAM command kinds.
const (
	cmdKindRead commandKind = iota
	cmdKindReadPrecharge
	cmdKindWrite
	cmdKindWritePrecharge
	cmdKindActivate
	cmdKindPrecharge
	cmdKindRefreshBank
	cmdKindRefresh
	cmdKindSRefEnter
	cmdKindSRefExit
	numCmdKind
)

// String returns the JEDEC-style mnemonic for a command kind. It labels the
// command-issue milestone on a sub-transaction's trace task.
func (k commandKind) String() string {
	switch k {
	case cmdKindRead:
		return "RD"
	case cmdKindReadPrecharge:
		return "RDA"
	case cmdKindWrite:
		return "WR"
	case cmdKindWritePrecharge:
		return "WRA"
	case cmdKindActivate:
		return "ACT"
	case cmdKindPrecharge:
		return "PRE"
	case cmdKindRefreshBank:
		return "REFb"
	case cmdKindRefresh:
		return "REF"
	case cmdKindSRefEnter:
		return "SREFE"
	case cmdKindSRefExit:
		return "SREFX"
	default:
		return "UNKNOWN"
	}
}

// location determines where to find the data to access.
type location struct {
	Channel   uint64 `json:"channel"`
	Rank      uint64 `json:"rank"`
	BankGroup uint64 `json:"bank_group"`
	Bank      uint64 `json:"bank"`
	Row       uint64 `json:"row"`
	Column    uint64 `json:"column"`
}

// bankStateKind represents the current state of a bank.
type bankStateKind int

// A list of possible bank states.
const (
	bankStateOpen bankStateKind = iota
	bankStateClosed
	bankStateSRef
	bankStatePD
	bankStateInvalid
)

// timeTableEntry is an entry in the timeTable.
type timeTableEntry struct {
	NextCmdKind       commandKind
	MinCycleInBetween int
}

// timeTable is a table that records the minimum number of cycles between any
// two types of DRAM commands.
type timeTable [][]timeTableEntry

// makeTimeTable creates a new timeTable.
func makeTimeTable() timeTable {
	return make([][]timeTableEntry, numCmdKind)
}

// dramTiming records all the timing-related parameters for a DRAM model.
type dramTiming struct {
	SameBank              timeTable
	OtherBanksInBankGroup timeTable
	SameRank              timeTable
	OtherRanks            timeTable
}

// State contains mutable runtime data for the DRAM memory controller.
type State struct {
	ControlState  memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID  uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort     `json:"current_cmd_src"`

	Transactions  []transactionState `json:"transactions"`
	SubTransQueue subTransQueueState `json:"sub_trans_queue"`
	CommandQueues commandQueueState  `json:"command_queues"`
	BankStates    bankStatesFlat     `json:"bank_states"`

	// PendingCompletions tracks issued read/write commands whose data/response
	// will become ready at a future tick. This timeline is decoupled from bank
	// occupancy: a bank can accept further (pipelined) column commands per the
	// timing table while earlier reads/writes are still returning data.
	PendingCompletions []pendingCompletion `json:"pending_completions"`

	// TickCount tracks the global cycle counter for tFAW enforcement.
	TickCount uint64 `json:"tick_count"`

	// RefreshCycleCounter counts cycles since last refresh.
	RefreshCycleCounter int `json:"refresh_cycle_counter"`
	// RefreshInProgress is true when a refresh is currently blocking.
	RefreshInProgress bool `json:"refresh_in_progress"`
	// RefreshCyclesRemaining counts remaining cycles of the current refresh.
	RefreshCyclesRemaining int `json:"refresh_cycles_remaining"`
	// RefreshBlockedIssue is set while a refresh window is holding off the issue
	// step (deviation D2: a global tRFC stall). It is cleared by the first
	// command that issues once the window ends, so that command's
	// sub-transaction can be charged a hardware_resource (refresh) milestone for
	// the otherwise-invisible stall.
	RefreshBlockedIssue bool `json:"refresh_blocked_issue"`

	// Statistics
	TotalReadCommands       uint64 `json:"total_read_commands"`
	TotalWriteCommands      uint64 `json:"total_write_commands"`
	TotalActivates          uint64 `json:"total_activates"`
	TotalPrecharges         uint64 `json:"total_precharges"`
	RowBufferHits           uint64 `json:"row_buffer_hits"`
	RowBufferMisses         uint64 `json:"row_buffer_misses"`
	TotalCycles             uint64 `json:"total_cycles"`
	TotalReadLatencyCycles  uint64 `json:"total_read_latency_cycles"`
	TotalWriteLatencyCycles uint64 `json:"total_write_latency_cycles"`
	CompletedReads          uint64 `json:"completed_reads"`
	CompletedWrites         uint64 `json:"completed_writes"`
	BytesRead               uint64 `json:"bytes_read"`
	BytesWritten            uint64 `json:"bytes_written"`
}

// subTransRef identifies a SubTransaction by its parent transaction's stable
// ID and its position within that transaction's SubTransactions slice. Using a
// stable ID (rather than a slice index) means removing a transaction never
// requires re-indexing the references held by queues, commands, or pending
// completions.
type subTransRef struct {
	TxID     uint64 `json:"tx_id"`
	SubIndex int    `json:"sub_index"`
}

// subTransState is a serializable representation of a SubTransaction.
type subTransState struct {
	ID        uint64 `json:"id"`
	Address   uint64 `json:"address"`
	Completed bool   `json:"completed"`
}

// pendingCompletion records that the read/write for a sub-transaction will have
// its data/response ready at CompletionTick. Stored in State, decoupled from
// the bank state machine.
type pendingCompletion struct {
	CompletionTick uint64      `json:"completion_tick"`
	Ref            subTransRef `json:"ref"`
}

// transactionState is a serializable representation of a Transaction.
type transactionState struct {
	ID              uint64               `json:"id"`
	HasRead         bool                 `json:"has_read"`
	HasWrite        bool                 `json:"has_write"`
	ReadMsg         memprotocol.ReadReq  `json:"read_msg"`
	WriteMsg        memprotocol.WriteReq `json:"write_msg"`
	SubTransactions []subTransState      `json:"sub_transactions"`
	ArrivalTick     uint64               `json:"arrival_tick"`
}

// commandState is a serializable representation of a Command.
type commandState struct {
	ID          uint64      `json:"id"`
	Kind        int         `json:"kind"`
	Address     uint64      `json:"address"`
	CycleLeft   int         `json:"cycle_left"`
	Location    location    `json:"location"`
	SubTransRef subTransRef `json:"sub_trans_ref"`
}

// bankEntry is a bankState tagged with its rank/bankGroup/bank indices.
type bankEntry struct {
	Rank      int       `json:"rank"`
	BankGroup int       `json:"bank_group"`
	BankIndex int       `json:"bank_index"`
	Data      bankState `json:"data"`
}

// bankState is a serializable representation of a Bank. It holds only the
// open/closed state machine and the per-next-command timing gaps. Bank
// occupancy is NOT tracked here: command eligibility is driven entirely by
// CyclesToCmdAvailable (the timing table) plus the state machine and tFAW.
// Data-return latency lives separately in State.PendingCompletions.
type bankState struct {
	State   int    `json:"state"`
	OpenRow uint64 `json:"open_row"`

	// CyclesToCmdAvailable[k] is the number of cycles before a command of kind
	// k may be issued to this bank. Indexed directly by commandKind.
	CyclesToCmdAvailable [numCmdKind]int `json:"cycles_to_cmd_available"`
}

// rankActivateHistory stores the last 4 activate timestamps for a rank.
type rankActivateHistory struct {
	Rank       int      `json:"rank"`
	Timestamps []uint64 `json:"timestamps"`
}

// bankStatesFlat is a flattened representation of the 3D bank array.
type bankStatesFlat struct {
	NumRanks          int                   `json:"num_ranks"`
	NumBankGroups     int                   `json:"num_bank_groups"`
	NumBanks          int                   `json:"num_banks"`
	Entries           []bankEntry           `json:"entries"`
	ActivateHistories []rankActivateHistory `json:"activate_histories"`
}

// queueEntry is a command state tagged with its queue index.
type queueEntry struct {
	QueueIndex int          `json:"queue_index"`
	Command    commandState `json:"command"`
	IsWrite    bool         `json:"is_write"`
}

// commandQueueState is a serializable representation of CommandQueues.
type commandQueueState struct {
	NumQueues      int          `json:"num_queues"`
	Entries        []queueEntry `json:"entries"`
	NextQueueIndex int          `json:"next_queue_index"`
	WriteDrainMode bool         `json:"write_drain_mode"`
}

// subTransQueueState is a list of sub-transaction references.
type subTransQueueState struct {
	Entries []subTransRef `json:"entries"`
}

// isTransactionCompleted checks if all sub-transactions of a transaction
// in the state are completed.
func isTransactionCompleted(t *transactionState) bool {
	for _, st := range t.SubTransactions {
		if !st.Completed {
			return false
		}
	}
	return true
}

// isTransactionRead returns true if the transaction is a read.
func isTransactionRead(t *transactionState) bool {
	return t.HasRead
}

// transactionGlobalAddress returns the address being accessed.
func transactionGlobalAddress(t *transactionState) uint64 {
	if t.HasRead {
		return t.ReadMsg.Address
	}
	return t.WriteMsg.Address
}

// transactionAccessByteSize returns number of bytes being accessed.
func transactionAccessByteSize(t *transactionState) uint64 {
	if t.HasRead {
		return t.ReadMsg.AccessByteSize
	}
	return uint64(len(t.WriteMsg.Data))
}

// initBankStatesFlat creates initial bank states for all banks (all closed).
// Entries are laid out rank-major, then bank-group, then bank, so a bank's
// position is computable directly (see bankFlatIndex) without a linear scan.
func initBankStatesFlat(numRanks, numBankGroups, numBanks int) bankStatesFlat {
	histories := make([]rankActivateHistory, numRanks)
	for i := range numRanks {
		histories[i] = rankActivateHistory{Rank: i}
	}

	flat := bankStatesFlat{
		NumRanks:          numRanks,
		NumBankGroups:     numBankGroups,
		NumBanks:          numBanks,
		Entries:           make([]bankEntry, 0, numRanks*numBankGroups*numBanks),
		ActivateHistories: histories,
	}

	for i := range numRanks {
		for j := range numBankGroups {
			for k := range numBanks {
				flat.Entries = append(flat.Entries, bankEntry{
					Rank:      i,
					BankGroup: j,
					BankIndex: k,
					Data: bankState{
						State: int(bankStateClosed),
					},
				})
			}
		}
	}

	return flat
}

// bankFlatIndex returns the index into bankStatesFlat.Entries for the bank at
// the given (rank, bankGroup, bank) coordinates, matching the layout produced
// by initBankStatesFlat.
func bankFlatIndex(flat *bankStatesFlat, rank, bankGroup, bank int) int {
	return (rank*flat.NumBankGroups+bankGroup)*flat.NumBanks + bank
}

// findBankState returns a pointer to the bankState for the given indices, or
// nil if the coordinates are out of range. O(1) — direct index, no scan.
func findBankState(flat *bankStatesFlat, rank, bankGroup, bank int) *bankState {
	idx := bankFlatIndex(flat, rank, bankGroup, bank)
	if idx < 0 || idx >= len(flat.Entries) {
		return nil
	}
	return &flat.Entries[idx].Data
}

// findTransaction returns the transaction with the given stable ID, or nil if
// it is not present (e.g. already completed and removed). The transaction list
// is bounded by TransactionQueueSize, so the scan is short.
func findTransaction(state *State, txID uint64) *transactionState {
	for i := range state.Transactions {
		if state.Transactions[i].ID == txID {
			return &state.Transactions[i]
		}
	}
	return nil
}

// Resources holds the shared resources referenced by the DRAM controller.
type Resources struct {
	Storage *mem.Storage
}

// Comp is the DRAM memory controller component.
type Comp = modeling.Component[Spec, State, Resources]
