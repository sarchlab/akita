package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
)

// State contains mutable runtime data for the DRAM memory controller.
type State struct {
	Transactions  []transactionState `json:"transactions"`
	SubTransQueue subTransQueueState `json:"sub_trans_queue"`
	CommandQueues commandQueueState  `json:"command_queues"`
	BankStates    bankStatesFlat     `json:"bank_states"`

	// TickCount tracks the global cycle counter for tFAW enforcement.
	TickCount uint64 `json:"tick_count"`

	// RefreshCycleCounter counts cycles since last refresh.
	RefreshCycleCounter int `json:"refresh_cycle_counter"`
	// RefreshInProgress is true when a refresh is currently blocking.
	RefreshInProgress bool `json:"refresh_in_progress"`
	// RefreshCyclesRemaining counts remaining cycles of the current refresh.
	RefreshCyclesRemaining int `json:"refresh_cycles_remaining"`

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
	ReadMsg         mem.ReadReq     `json:"read_msg"`
	WriteMsg        mem.WriteReq    `json:"write_msg"`
	InternalAddress uint64          `json:"internal_address"`
	SubTransactions []subTransState `json:"sub_transactions"`
}

// commandState is a serializable representation of a Command.
type commandState struct {
	ID          string      `json:"id"`
	Kind        int         `json:"kind"`
	Address     uint64      `json:"address"`
	CycleLeft   int         `json:"cycle_left"`
	Location    Location    `json:"location"`
	SubTransRef subTransRef `json:"sub_trans_ref"`
}

// bankEntry is a bankState tagged with its rank/bankGroup/bank indices.
type bankEntry struct {
	Rank      int       `json:"rank"`
	BankGroup int       `json:"bank_group"`
	BankIndex int       `json:"bank_index"`
	Data      bankState `json:"data"`
}

// bankState is a serializable representation of a Bank.
type bankState struct {
	State                int            `json:"state"`
	OpenRow              uint64         `json:"open_row"`
	HasCurrentCmd        bool           `json:"has_current_cmd"`
	CurrentCmd           commandState   `json:"current_cmd"`
	CyclesToCmdAvailable map[string]int `json:"cycles_to_cmd_available"`
}

// rankActivateHistory stores the last 4 activate timestamps for a rank.
type rankActivateHistory struct {
	Rank       int    `json:"rank"`
	Timestamps []uint64 `json:"timestamps"`
}

// bankStatesFlat is a flattened representation of the 3D bank array.
type bankStatesFlat struct {
	NumRanks           int                    `json:"num_ranks"`
	NumBankGroups      int                    `json:"num_bank_groups"`
	NumBanks           int                    `json:"num_banks"`
	Entries            []bankEntry            `json:"entries"`
	ActivateHistories  []rankActivateHistory  `json:"activate_histories"`
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

// cmdKindToString converts a CommandKind int to string key.
func cmdKindToString(k CommandKind) string {
	return fmt.Sprintf("%d", int(k))
}

// initBankStatesFlat creates initial bank states for all banks (all closed).
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
						State:                int(BankStateClosed),
						CyclesToCmdAvailable: make(map[string]int),
					},
				})
			}
		}
	}

	return flat
}

// findBankState returns a pointer to the bankState for the given indices.
func findBankState(flat *bankStatesFlat, rank, bankGroup, bank int) *bankState {
	for i := range flat.Entries {
		e := &flat.Entries[i]
		if e.Rank == rank && e.BankGroup == bankGroup && e.BankIndex == bank {
			return &e.Data
		}
	}
	return nil
}
