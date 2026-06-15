package dram

// tickBanks counts down the per-next-command timing gaps on every bank.
// Returns true if any gap was decremented.
func tickBanks(state *State) bool {
	madeProgress := false

	for i := range state.BankStates.Entries {
		bs := &state.BankStates.Entries[i].Data
		madeProgress = tickBank(bs) || madeProgress
	}

	return madeProgress
}

// tickBank counts down a single bank's timing constraints.
func tickBank(bs *bankState) bool {
	madeProgress := false

	for k := range bs.CyclesToCmdAvailable {
		if bs.CyclesToCmdAvailable[k] > 0 {
			bs.CyclesToCmdAvailable[k]--
			madeProgress = true
		}
	}

	return madeProgress
}

// processPendingCompletions marks every sub-transaction whose read/write data
// has become ready (its CompletionTick has been reached) as completed, then
// drops those entries. Returns true if any sub-transaction completed.
//
// This is the data-return timeline, decoupled from bank occupancy: multiple
// reads/writes may be in flight on the same bank, each completing on its own
// schedule, while the bank accepts further column commands per the timing table.
func processPendingCompletions(state *State) bool {
	if len(state.PendingCompletions) == 0 {
		return false
	}

	progress := false
	kept := state.PendingCompletions[:0]
	for _, pc := range state.PendingCompletions {
		if pc.CompletionTick <= state.TickCount {
			markSubTransCompleted(state, pc.Ref)
			progress = true
			continue
		}
		kept = append(kept, pc)
	}
	state.PendingCompletions = kept

	return progress
}

// markSubTransCompleted flags the referenced sub-transaction as completed. A
// stale reference (parent transaction already removed) is a safe no-op.
func markSubTransCompleted(state *State, ref subTransRef) {
	t := findTransaction(state, ref.TxID)
	if t == nil {
		return
	}
	if ref.SubIndex >= 0 && ref.SubIndex < len(t.SubTransactions) {
		t.SubTransactions[ref.SubIndex].Completed = true
	}
}

// isReadOrWrite returns true if the command kind is a read/write variant.
func isReadOrWrite(kind commandKind) bool {
	return kind == cmdKindRead || kind == cmdKindReadPrecharge ||
		kind == cmdKindWrite || kind == cmdKindWritePrecharge
}

// getReadyCommand checks if a command can be issued to the bank.
// It returns a copy of the command with the required kind, or nil.
func getReadyCommand(spec *Spec, state *State, bs *bankState, cmd *commandState) *commandState {
	requiredKind := getRequiredCommandKind(bs, cmd)
	if requiredKind == numCmdKind {
		return nil
	}

	if bs.CyclesToCmdAvailable[requiredKind] == 0 {
		// Check tFAW for activate commands
		if requiredKind == cmdKindActivate && spec.TFAW > 0 {
			if !canActivateUnderTFAW(spec, state, int(cmd.Location.Rank)) {
				return nil
			}
		}
		readyCmd := cloneCommand(cmd)
		readyCmd.Kind = int(requiredKind)
		return readyCmd
	}

	return nil
}

// canActivateUnderTFAW checks whether issuing an activate on the given rank
// would violate the tFAW constraint.
func canActivateUnderTFAW(spec *Spec, state *State, rank int) bool {
	history := findActivateHistory(&state.BankStates, rank)
	if history == nil {
		return true
	}
	stamps := history.Timestamps
	if len(stamps) < 4 {
		return true
	}
	// Check if the oldest of the last 4 activates is more than tFAW ago
	oldest := stamps[len(stamps)-4]
	return state.TickCount-oldest >= uint64(spec.TFAW)
}

// findActivateHistory returns a pointer to the rankActivateHistory for the
// given rank, or nil if out of range. Histories are pre-allocated one per rank
// (see initBankStatesFlat), so the rank is a direct index.
func findActivateHistory(flat *bankStatesFlat, rank int) *rankActivateHistory {
	if rank < 0 || rank >= len(flat.ActivateHistories) {
		return nil
	}
	return &flat.ActivateHistories[rank]
}

// getRequiredCommandKind determines what command kind is actually needed
// given the bank state and the requested command.
func getRequiredCommandKind(bs *bankState, cmd *commandState) commandKind {
	bankSt := bankStateKind(bs.State)
	cmdKind := commandKind(cmd.Kind)

	type key struct {
		state bankStateKind
		kind  commandKind
	}

	switch (key{bankSt, cmdKind}) {
	case key{bankStateClosed, cmdKindRead},
		key{bankStateClosed, cmdKindReadPrecharge},
		key{bankStateClosed, cmdKindWrite},
		key{bankStateClosed, cmdKindWritePrecharge}:
		return cmdKindActivate

	case key{bankStateOpen, cmdKindRead},
		key{bankStateOpen, cmdKindReadPrecharge},
		key{bankStateOpen, cmdKindWrite},
		key{bankStateOpen, cmdKindWritePrecharge}:
		if bs.OpenRow == cmd.Location.Row {
			return cmdKind
		}
		return cmdKindPrecharge

	default:
		return numCmdKind
	}
}

// startCommand issues a command to a bank. It updates the bank state machine,
// statistics, and — for column reads/writes — schedules the data/response to
// become ready on the pending-completion timeline. It does NOT mark the bank
// "busy": next-command eligibility is governed solely by the timing table
// (CyclesToCmdAvailable), the state machine, and tFAW, which is what allows
// pipelined column commands without conflating bus occupancy with data latency.
func startCommand(cmdCycles map[commandKind]int, state *State, bs *bankState, cmd *commandState) {
	kind := commandKind(cmd.Kind)

	if isReadOrWrite(kind) {
		delay := cmdCycles[kind]
		state.PendingCompletions = append(
			state.PendingCompletions,
			pendingCompletion{
				CompletionTick: state.TickCount + uint64(delay),
				Ref:            cmd.SubTransRef,
			},
		)
	}

	// Track statistics
	switch kind {
	case cmdKindRead, cmdKindReadPrecharge:
		state.TotalReadCommands++
	case cmdKindWrite, cmdKindWritePrecharge:
		state.TotalWriteCommands++
	case cmdKindActivate:
		state.TotalActivates++
	case cmdKindPrecharge:
		state.TotalPrecharges++
	}

	// Update bank state based on the command
	bankSt := bankStateKind(bs.State)

	type key struct {
		state bankStateKind
		kind  commandKind
	}

	switch (key{bankSt, kind}) {
	case key{bankStateClosed, cmdKindActivate}:
		bs.OpenRow = cmd.Location.Row
		bs.State = int(bankStateOpen)
		recordActivateTimestamp(state, int(cmd.Location.Rank))
	case key{bankStateOpen, cmdKindPrecharge},
		key{bankStateOpen, cmdKindReadPrecharge},
		key{bankStateOpen, cmdKindWritePrecharge}:
		bs.State = int(bankStateClosed)
	case key{bankStateOpen, cmdKindRead},
		key{bankStateOpen, cmdKindWrite}:
		// Do nothing
	}
}

// updateTiming updates timing constraints across all banks after a command
// is issued.
func updateTiming(timing dramTiming, state *State, cmd *commandState) {
	kind := commandKind(cmd.Kind)

	switch kind {
	case cmdKindActivate,
		cmdKindRead, cmdKindReadPrecharge,
		cmdKindWrite, cmdKindWritePrecharge,
		cmdKindPrecharge, cmdKindRefreshBank:
		updateAllBankTiming(timing, state, cmd)
	}
}

// updateAllBankTiming iterates over all banks and applies timing constraints.
func updateAllBankTiming(timing dramTiming, state *State, cmd *commandState) {
	kind := commandKind(cmd.Kind)
	flat := &state.BankStates

	for i := range flat.Entries {
		entry := &flat.Entries[i]
		rank := uint64(entry.Rank)
		bankGroup := uint64(entry.BankGroup)
		bank := uint64(entry.BankIndex)

		var timingTable timeTable
		if cmd.Location.Rank == rank {
			if cmd.Location.BankGroup == bankGroup {
				if cmd.Location.Bank == bank {
					timingTable = timing.SameBank
				} else {
					timingTable = timing.OtherBanksInBankGroup
				}
			} else {
				timingTable = timing.SameRank
			}
		} else {
			timingTable = timing.OtherRanks
		}

		if int(kind) < len(timingTable) {
			for _, te := range timingTable[kind] {
				if entry.Data.CyclesToCmdAvailable[te.NextCmdKind] <
					te.MinCycleInBetween {
					entry.Data.CyclesToCmdAvailable[te.NextCmdKind] =
						te.MinCycleInBetween
				}
			}
		}
	}
}

// recordActivateTimestamp records the current tick as an activate timestamp
// for the given rank and keeps only the last 4 (the tFAW window).
func recordActivateTimestamp(state *State, rank int) {
	history := findActivateHistory(&state.BankStates, rank)
	if history == nil {
		return
	}
	history.Timestamps = append(history.Timestamps, state.TickCount)
	// Keep only the last 4
	if len(history.Timestamps) > 4 {
		history.Timestamps = history.Timestamps[len(history.Timestamps)-4:]
	}
}

// cloneCommand creates a copy of a commandState with the same content.
func cloneCommand(cmd *commandState) *commandState {
	c := *cmd
	return &c
}
