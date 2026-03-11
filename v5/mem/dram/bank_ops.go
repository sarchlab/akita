package dram

// tickBanks updates all bank states: counts down timers and completes commands.
// Returns true if any progress was made.
func tickBanks(spec *Spec, state *State) bool {
	madeProgress := false

	for i := range state.BankStates.Entries {
		bs := &state.BankStates.Entries[i].Data
		madeProgress = tickBank(spec, state, bs) || madeProgress
	}

	return madeProgress
}

// tickBank updates a single bank's state.
func tickBank(spec *Spec, state *State, bs *bankState) bool {
	madeProgress := false

	// Count down current command
	if bs.HasCurrentCmd {
		bs.CurrentCmd.CycleLeft--
		if bs.CurrentCmd.CycleLeft <= 0 {
			completeCommand(state, bs)
		}
		madeProgress = true
	}

	// Count down timing constraints
	for k, v := range bs.CyclesToCmdAvailable {
		if v > 0 {
			bs.CyclesToCmdAvailable[k] = v - 1
			madeProgress = true
		}
	}

	return madeProgress
}

// completeCommand finishes the current command on a bank.
func completeCommand(state *State, bs *bankState) {
	cmd := &bs.CurrentCmd
	cmd.CycleLeft = 0

	kind := CommandKind(cmd.Kind)
	if isReadOrWrite(kind) {
		// Mark the sub-transaction as completed
		ref := cmd.SubTransRef
		state.Transactions[ref.TransIndex].
			SubTransactions[ref.SubIndex].Completed = true
	}

	bs.HasCurrentCmd = false
	bs.CurrentCmd = commandState{}
}

// isReadOrWrite returns true if the command kind is a read/write variant.
func isReadOrWrite(kind CommandKind) bool {
	return kind == CmdKindRead || kind == CmdKindReadPrecharge ||
		kind == CmdKindWrite || kind == CmdKindWritePrecharge
}

// getReadyCommand checks if a command can be issued to the bank.
// It returns a copy of the command with the required kind, or nil.
func getReadyCommand(spec *Spec, bs *bankState, cmd *commandState) *commandState {
	requiredKind := getRequiredCommandKind(bs, cmd)
	if requiredKind == NumCmdKind {
		return nil
	}

	key := cmdKindToString(requiredKind)
	if bs.CyclesToCmdAvailable[key] == 0 {
		readyCmd := cloneCommand(cmd)
		readyCmd.Kind = int(requiredKind)
		return readyCmd
	}

	return nil
}

// getRequiredCommandKind determines what command kind is actually needed
// given the bank state and the requested command.
func getRequiredCommandKind(bs *bankState, cmd *commandState) CommandKind {
	bankSt := BankStateKind(bs.State)
	cmdKind := CommandKind(cmd.Kind)

	type key struct {
		state BankStateKind
		kind  CommandKind
	}

	switch (key{bankSt, cmdKind}) {
	case key{BankStateClosed, CmdKindRead},
		key{BankStateClosed, CmdKindReadPrecharge},
		key{BankStateClosed, CmdKindWrite},
		key{BankStateClosed, CmdKindWritePrecharge}:
		return CmdKindActivate

	case key{BankStateOpen, CmdKindRead},
		key{BankStateOpen, CmdKindReadPrecharge},
		key{BankStateOpen, CmdKindWrite},
		key{BankStateOpen, CmdKindWritePrecharge}:
		if bs.OpenRow == cmd.Location.Row {
			return cmdKind
		}
		return CmdKindPrecharge

	default:
		return NumCmdKind
	}
}

// startCommand starts a command on a bank.
func startCommand(spec *Spec, state *State, bs *bankState, cmd *commandState) {
	if bs.HasCurrentCmd {
		panic("previous cmd is not completed")
	}

	bs.HasCurrentCmd = true
	bs.CurrentCmd = *cmd

	kind := CommandKind(cmd.Kind)
	cycles, ok := spec.CmdCycles[kind]
	if ok {
		bs.CurrentCmd.CycleLeft = cycles
	}

	// Update bank state based on the command
	bankSt := BankStateKind(bs.State)

	type key struct {
		state BankStateKind
		kind  CommandKind
	}

	switch (key{bankSt, kind}) {
	case key{BankStateClosed, CmdKindActivate}:
		bs.OpenRow = cmd.Location.Row
		bs.State = int(BankStateOpen)
	case key{BankStateOpen, CmdKindPrecharge},
		key{BankStateOpen, CmdKindReadPrecharge},
		key{BankStateOpen, CmdKindWritePrecharge}:
		bs.State = int(BankStateClosed)
	case key{BankStateOpen, CmdKindRead},
		key{BankStateOpen, CmdKindWrite}:
		// Do nothing
	}
}

// updateTiming updates timing constraints across all banks after a command
// is issued.
func updateTiming(spec *Spec, state *State, cmd *commandState) {
	kind := CommandKind(cmd.Kind)

	switch kind {
	case CmdKindActivate,
		CmdKindRead, CmdKindReadPrecharge,
		CmdKindWrite, CmdKindWritePrecharge,
		CmdKindPrecharge, CmdKindRefreshBank:
		updateAllBankTiming(spec, state, cmd)
	}
}

// updateAllBankTiming iterates over all banks and applies timing constraints.
func updateAllBankTiming(spec *Spec, state *State, cmd *commandState) {
	kind := CommandKind(cmd.Kind)
	flat := &state.BankStates

	for i := range flat.Entries {
		entry := &flat.Entries[i]
		rank := uint64(entry.Rank)
		bankGroup := uint64(entry.BankGroup)
		bank := uint64(entry.BankIndex)

		var timingTable TimeTable
		if cmd.Location.Rank == rank {
			if cmd.Location.BankGroup == bankGroup {
				if cmd.Location.Bank == bank {
					timingTable = spec.Timing.SameBank
				} else {
					timingTable = spec.Timing.OtherBanksInBankGroup
				}
			} else {
				timingTable = spec.Timing.SameRank
			}
		} else {
			timingTable = spec.Timing.OtherRanks
		}

		if int(kind) < len(timingTable) {
			for _, te := range timingTable[kind] {
				key := cmdKindToString(te.NextCmdKind)
				if entry.Data.CyclesToCmdAvailable == nil {
					entry.Data.CyclesToCmdAvailable = make(map[string]int)
				}
				if entry.Data.CyclesToCmdAvailable[key] < te.MinCycleInBetween {
					entry.Data.CyclesToCmdAvailable[key] = te.MinCycleInBetween
				}
			}
		}
	}
}

// cloneCommand creates a copy of a commandState with the same content.
func cloneCommand(cmd *commandState) *commandState {
	c := *cmd
	return &c
}
