package dram

import (
	"github.com/sarchlab/akita/v5/timing"
)

// splitTransaction breaks a transaction into sub-transactions based on
// the access unit size (from Spec.Log2AccessUnitSize).
func splitTransaction(
	spec *Spec,
	trans *transactionState,
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
			ID:        timing.GetIDGenerator().Generate(),
			Address:   a,
			Completed: false,
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
// queue, referenced by the transaction's stable ID.
func pushSubTrans(state *State, transIdx int) {
	trans := &state.Transactions[transIdx]
	for i := range trans.SubTransactions {
		state.SubTransQueue.Entries = append(
			state.SubTransQueue.Entries,
			subTransRef{TxID: trans.ID, SubIndex: i},
		)
	}
}

// tickSubTransQueue tries to move one sub-transaction from the queue into the
// command queue using the default plugin set. Returns true if progress was
// made. Production drives this through the component's configured controller
// (see controller.fillCommandQueue); this package-level shim builds the default
// controller so tests can exercise the path directly.
func tickSubTransQueue(spec *Spec, state *State) bool {
	return newDefaultController(spec).fillCommandQueue(spec, state)
}

// createClosePageCommand creates a command for a sub-transaction using
// close-page policy (auto-precharge). Thin wrapper over the row policy, kept
// for direct testing.
func createClosePageCommand(
	spec *Spec,
	state *State,
	ref subTransRef,
) *commandState {
	st := subTransByRef(state, ref)
	return closePageRowPolicy{}.CommandFor(
		spec, state, ref, mapAddress(spec, st.Address))
}

// createOpenPageCommand creates a command for a sub-transaction using
// open-page policy (plain Read/Write, leaving the row buffer open). Thin
// wrapper over the row policy, kept for direct testing.
func createOpenPageCommand(
	spec *Spec,
	state *State,
	ref subTransRef,
) *commandState {
	st := subTransByRef(state, ref)
	return openPageRowPolicy{}.CommandFor(
		spec, state, ref, mapAddress(spec, st.Address))
}

// getQueueIndex returns the command-queue index for a command. The mapping is
// meant to depend on the configured queue structure (PER_RANK groups a rank's
// banks into one queue; PER_BANK gives each bank its own queue), but only
// PER_RANK is implemented so far — PER_BANK is the subject of the failing tests
// in perbank_queue_test.go.
func getQueueIndex(spec *Spec, cmd *commandState) int {
	return int(cmd.Location.Rank)
}

// isWriteCommand returns true if the command is a write or write-precharge.
func isWriteCommand(cmd *commandState) bool {
	kind := commandKind(cmd.Kind)
	return kind == cmdKindWrite || kind == cmdKindWritePrecharge
}

// canAcceptCommand returns true if there is space in the command queue for
// the command. When read/write queue separation is configured (sizes > 0),
// it checks read and write capacities separately.
func canAcceptCommand(
	state *State,
	cmd *commandState,
	spec *Spec,
) bool {
	queueIdx := getQueueIndex(spec, cmd)
	isWrite := isWriteCommand(cmd)

	// If R/W queue separation is configured (sizes > 0), use separate limits
	if spec.ReadQueueSize > 0 && spec.WriteQueueSize > 0 {
		count := 0
		for _, e := range state.CommandQueues.Entries {
			if e.QueueIndex == queueIdx && e.IsWrite == isWrite {
				count++
			}
		}
		if isWrite {
			return count < spec.WriteQueueSize
		}
		return count < spec.ReadQueueSize
	}

	// Fallback to unified capacity
	count := 0
	for _, e := range state.CommandQueues.Entries {
		if e.QueueIndex == queueIdx {
			count++
		}
	}
	return count < spec.CommandQueueCapacity
}

// acceptCommand adds a command to the command queue.
func acceptCommand(state *State, cmd *commandState, spec *Spec) {
	queueIdx := getQueueIndex(spec, cmd)
	state.CommandQueues.Entries = append(
		state.CommandQueues.Entries,
		queueEntry{
			QueueIndex: queueIdx,
			Command:    *cmd,
			IsWrite:    isWriteCommand(cmd),
		},
	)
}

// getCommandToIssue uses FR-FCFS (First-Ready First-Come-First-Served)
// scheduling. When read/write queue separation is configured, it implements
// write drain mode: once the number of pending write commands reaches the
// high watermark, the controller drains writes until the count drops to
// the low watermark. It prioritises row-buffer hits (bank is open, matching
// row, and the command is ready) over other ready commands. Among commands
// of equal priority, the oldest (earliest in the queue) wins.
func getCommandToIssue(spec *Spec, next *State) *commandState {
	// Write drain logic (only when R/W queue separation is configured)
	if spec.ReadQueueSize > 0 && spec.WriteQueueSize > 0 {
		writeCount := countWriteCommands(next)
		if !next.CommandQueues.WriteDrainMode &&
			writeCount >= spec.WriteHighWatermark {
			next.CommandQueues.WriteDrainMode = true
		}
		if next.CommandQueues.WriteDrainMode &&
			writeCount <= spec.WriteLowWatermark {
			next.CommandQueues.WriteDrainMode = false
		}

		if next.CommandQueues.WriteDrainMode {
			cmd := getFirstReadyWrite(spec, next)
			if cmd != nil {
				return cmd
			}
		}
	}

	// Priority 1: Find a row-buffer hit (bank is open, matching row, command is ready)
	hit := findRowBufferHitCommand(spec, next)
	if hit != nil {
		next.RowBufferHits++
		return hit
	}

	// Priority 2: FCFS — oldest ready command (any)
	miss := findOldestReadyCommand(spec, next)
	if miss != nil {
		next.RowBufferMisses++
		return miss
	}

	return nil
}

// findRowBufferHitCommand scans the command queue for a row-buffer hit
// (bank is open, matching row, and the command is ready). Returns the first
// (oldest) such command, or nil if none found.
func findRowBufferHitCommand(spec *Spec, next *State) *commandState {
	for i := range next.CommandQueues.Entries {
		e := &next.CommandQueues.Entries[i]
		cmd := &e.Command
		bs := findBankStateByLocation(&next.BankStates, cmd.Location)
		if bs == nil {
			continue
		}
		if bankStateKind(bs.State) == bankStateOpen && bs.OpenRow == cmd.Location.Row {
			readyCmd := getReadyCommand(spec, next, bs, cmd)
			if readyCmd != nil {
				if readyCmd.Kind == cmd.Kind {
					removeCommandFromQueueByIndex(next, i)
				}
				return readyCmd
			}
		}
	}

	return nil
}

// findOldestReadyCommand scans the command queue for the oldest ready command
// regardless of row-buffer state. Returns the first ready command, or nil.
func findOldestReadyCommand(spec *Spec, next *State) *commandState {
	for i := range next.CommandQueues.Entries {
		e := &next.CommandQueues.Entries[i]
		cmd := &e.Command
		bs := findBankStateByLocation(&next.BankStates, cmd.Location)
		if bs == nil {
			continue
		}
		readyCmd := getReadyCommand(spec, next, bs, cmd)
		if readyCmd != nil {
			if readyCmd.Kind == cmd.Kind {
				removeCommandFromQueueByIndex(next, i)
			}
			return readyCmd
		}
	}

	return nil
}

// removeCommandFromQueueByIndex removes a command entry at the given index
// from the command queue.
func removeCommandFromQueueByIndex(next *State, idx int) {
	next.CommandQueues.Entries = append(
		next.CommandQueues.Entries[:idx],
		next.CommandQueues.Entries[idx+1:]...,
	)
}

// findBankStateByLocation finds the bank state for a given Location.
func findBankStateByLocation(flat *bankStatesFlat, loc location) *bankState {
	return findBankState(flat,
		int(loc.Rank), int(loc.BankGroup), int(loc.Bank))
}

// countWriteCommands returns the total number of write commands in the
// command queue.
func countWriteCommands(state *State) int {
	count := 0
	for _, e := range state.CommandQueues.Entries {
		if e.IsWrite {
			count++
		}
	}
	return count
}

// getFirstReadyWrite returns the first ready write command from the queue,
// using FR-FCFS ordering (row-buffer hits first, then oldest).
func getFirstReadyWrite(spec *Spec, next *State) *commandState {
	// First pass: look for a row-buffer hit among writes
	for i := range next.CommandQueues.Entries {
		e := &next.CommandQueues.Entries[i]
		if !e.IsWrite {
			continue
		}
		cmd := &e.Command
		bs := findBankStateByLocation(&next.BankStates, cmd.Location)
		if bs == nil {
			continue
		}
		if bankStateKind(bs.State) == bankStateOpen &&
			bs.OpenRow == cmd.Location.Row {
			readyCmd := getReadyCommand(spec, next, bs, cmd)
			if readyCmd != nil {
				if readyCmd.Kind == cmd.Kind {
					removeCommandFromQueueByIndex(next, i)
				}
				return readyCmd
			}
		}
	}

	// Second pass: oldest ready write (any bank state)
	for i := range next.CommandQueues.Entries {
		e := &next.CommandQueues.Entries[i]
		if !e.IsWrite {
			continue
		}
		cmd := &e.Command
		bs := findBankStateByLocation(&next.BankStates, cmd.Location)
		if bs == nil {
			continue
		}
		readyCmd := getReadyCommand(spec, next, bs, cmd)
		if readyCmd != nil {
			if readyCmd.Kind == cmd.Kind {
				removeCommandFromQueueByIndex(next, i)
			}
			return readyCmd
		}
	}

	return nil
}
