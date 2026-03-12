package dram

import (
	"github.com/sarchlab/akita/v5/sim"
)

// splitTransaction breaks a transaction into sub-transactions based on
// the access unit size (from Spec.Log2AccessUnitSize).
func splitTransaction(
	spec *Spec,
	trans *transactionState,
	transIdx int,
) {
	addr := transactionGlobalAddress(trans)
	size := transactionAccessByteSize(trans)

	unitSize := uint64(1 << spec.Log2AccessUnitSize)

	// Align
	addrMask := ^(unitSize - 1)
	alignedAddr := addr & addrMask
	endAddr := addr + size

	currAddr := alignedAddr
	var alignedSize uint64
	for currAddr < endAddr {
		alignedSize += unitSize
		currAddr += unitSize
	}

	alignedEnd := alignedAddr + alignedSize

	for a := alignedAddr; a < alignedEnd; a += unitSize {
		st := subTransState{
			ID:               sim.GetIDGenerator().Generate(),
			Address:          a,
			Completed:        false,
			TransactionIndex: transIdx,
		}
		trans.SubTransactions = append(trans.SubTransactions, st)
	}
}

// canPushSubTrans returns true if the subtrans queue can hold n more entries.
func canPushSubTrans(state *State, n int, capacity int) bool {
	if n >= capacity {
		panic("queue size not large enough to handle a single transaction")
	}
	return len(state.SubTransQueue.Entries)+n <= capacity
}

// pushSubTrans adds all subtransactions of a transaction to the sub-transaction
// queue.
func pushSubTrans(state *State, transIdx int) {
	trans := &state.Transactions[transIdx]
	for i := range trans.SubTransactions {
		state.SubTransQueue.Entries = append(
			state.SubTransQueue.Entries,
			subTransRef{TransIndex: transIdx, SubIndex: i},
		)
	}
}

// tickSubTransQueue tries to move one sub-transaction from the queue into
// the command queue. Returns true if progress was made.
func tickSubTransQueue(spec *Spec, state *State) bool {
	for i, ref := range state.SubTransQueue.Entries {
		cmd := createClosePageCommand(spec, state, ref)

		if canAcceptCommand(state, cmd, spec.CommandQueueCapacity) {
			acceptCommand(state, cmd)
			// Remove from queue
			state.SubTransQueue.Entries = append(
				state.SubTransQueue.Entries[:i],
				state.SubTransQueue.Entries[i+1:]...,
			)
			return true
		}
	}

	return false
}

// createClosePageCommand creates a command for a sub-transaction using
// close-page policy.
func createClosePageCommand(
	spec *Spec,
	state *State,
	ref subTransRef,
) *commandState {
	st := &state.Transactions[ref.TransIndex].SubTransactions[ref.SubIndex]

	cmd := &commandState{
		ID:      sim.GetIDGenerator().Generate(),
		Address: st.Address,
		SubTransRef: subTransRef{
			TransIndex: ref.TransIndex,
			SubIndex:   ref.SubIndex,
		},
	}

	// Close-page: read => ReadPrecharge, write => WritePrecharge
	trans := &state.Transactions[ref.TransIndex]
	if isTransactionRead(trans) {
		cmd.Kind = int(CmdKindReadPrecharge)
	} else {
		cmd.Kind = int(CmdKindWritePrecharge)
	}

	loc := mapAddress(spec, st.Address)
	cmd.Location = loc

	return cmd
}

// getQueueIndex returns the command queue index for a command (by rank).
func getQueueIndex(cmd *commandState) int {
	return int(cmd.Location.Rank)
}

// canAcceptCommand returns true if there is space in the command queue for
// the command.
func canAcceptCommand(
	state *State,
	cmd *commandState,
	capacityPerQueue int,
) bool {
	queueIdx := getQueueIndex(cmd)
	count := 0
	for _, e := range state.CommandQueues.Entries {
		if e.QueueIndex == queueIdx {
			count++
		}
	}
	return count < capacityPerQueue
}

// acceptCommand adds a command to the command queue.
func acceptCommand(state *State, cmd *commandState) {
	queueIdx := getQueueIndex(cmd)
	state.CommandQueues.Entries = append(
		state.CommandQueues.Entries,
		queueEntry{QueueIndex: queueIdx, Command: *cmd},
	)
}

// getCommandToIssue iterates over command queues round-robin and returns
// the first ready command. Operates on next state only (which has already
// been updated by respondMW).
// Returns nil if none is ready.
func getCommandToIssue(spec *Spec, next *State) *commandState {
	numQueues := next.CommandQueues.NumQueues
	if numQueues == 0 {
		return nil
	}

	startIdx := next.CommandQueues.NextQueueIndex
	for i := 0; i < numQueues; i++ {
		queueIdx := (startIdx + i) % numQueues
		next.CommandQueues.NextQueueIndex = (queueIdx + 1) % numQueues

		readyCmd := getFirstReadyInQueue(spec, next, queueIdx)
		if readyCmd != nil {
			return readyCmd
		}
	}

	return nil
}

// getFirstReadyInQueue finds the first command in a specific queue that
// can be issued. Operates on next state only.
func getFirstReadyInQueue(
	spec *Spec,
	next *State,
	queueIdx int,
) *commandState {
	for i := 0; i < len(next.CommandQueues.Entries); i++ {
		e := &next.CommandQueues.Entries[i]
		if e.QueueIndex != queueIdx {
			continue
		}

		cmd := &e.Command
		bs := findBankStateByLocation(&next.BankStates, cmd.Location)
		if bs == nil {
			continue
		}

		readyCmd := getReadyCommand(spec, bs, cmd)
		if readyCmd != nil {
			// If the ready command kind matches the original, remove from next
			if cmd.Kind == readyCmd.Kind {
				removeCommandFromQueueByID(next, cmd.ID)
			}
			return readyCmd
		}
	}

	return nil
}

// removeCommandFromQueueByID removes a command entry with matching ID from
// next's command queue.
func removeCommandFromQueueByID(next *State, cmdID string) {
	for i := 0; i < len(next.CommandQueues.Entries); i++ {
		if next.CommandQueues.Entries[i].Command.ID == cmdID {
			next.CommandQueues.Entries = append(
				next.CommandQueues.Entries[:i],
				next.CommandQueues.Entries[i+1:]...,
			)
			return
		}
	}
}

// findBankStateByLocation finds the bank state for a given Location.
func findBankStateByLocation(flat *bankStatesFlat, loc Location) *bankState {
	return findBankState(flat,
		int(loc.Rank), int(loc.BankGroup), int(loc.Bank))
}
