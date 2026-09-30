package dram

// buildCmdCycles returns the data-return (completion) timeline used by
// startCommand: how long after a column command its read data / write
// response is ready. For the auto-precharge variants the data still returns
// readDelay/writeDelay after the column command — the trailing precharge is
// enforced separately by the bank timing table, so it must NOT shorten the
// completion to tRP.
func buildCmdCycles(spec *Spec) map[commandKind]int {
	proto := protocol(spec.Protocol)

	cmdCycles := map[commandKind]int{
		cmdKindRead:           spec.readDelay(),
		cmdKindReadPrecharge:  spec.readDelay(),
		cmdKindWrite:          spec.writeDelay(),
		cmdKindWritePrecharge: spec.writeDelay(),
		cmdKindActivate:       spec.TRCD - spec.TAL,
		cmdKindPrecharge:      spec.TRP,
		cmdKindRefreshBank:    1,
		cmdKindRefresh:        1,
		cmdKindSRefEnter:      1,
		cmdKindSRefExit:       1,
	}

	if proto.isGDDR() || proto.isHBM() {
		cmdCycles[cmdKindActivate] = spec.TRCDRD - spec.TAL
	}

	return cmdCycles
}

// generateTiming returns the minimum number of cycles between any two
// commands, for a command to the same bank, another bank in the same bank
// group, the same rank, and another rank.
//
//nolint:gocyclo,funlen
func generateTiming(s *Spec) dramTiming {
	proto := protocol(s.Protocol)

	burstCycle := s.burstCycle()
	tRL := s.tRL()
	tWL := s.tWL()
	readDelay := s.readDelay()
	writeDelay := s.writeDelay()

	t := dramTiming{
		SameBank:              makeTimeTable(),
		OtherBanksInBankGroup: makeTimeTable(),
		SameRank:              makeTimeTable(),
		OtherRanks:            makeTimeTable(),
	}

	readToReadL := max(burstCycle, s.TCCDL)
	readToReadS := max(burstCycle, s.TCCDS)
	readToReadO := burstCycle + s.TRTRS
	readToWrite := tRL + burstCycle - tWL + s.TRTRS
	readToWriteO := readDelay + burstCycle +
		s.TRTRS - writeDelay
	readToPrecharge := s.TAL + s.TRTP
	readpToAct := s.TAL + burstCycle + s.TRTP + s.TRP

	writeToReadL := writeDelay + s.TWTRL
	writeToReadS := writeDelay + s.TWTRS
	writeToReadO := writeDelay + burstCycle +
		s.TRTRS - readDelay
	writeToWriteL := max(burstCycle, s.TCCDL)
	writeToWriteS := max(burstCycle, s.TCCDS)
	writeToWriteO := burstCycle
	writeToPrecharge := tWL + burstCycle + s.TWR

	prechargeToActivate := s.TRP
	prechargeToPrecharge := s.TPPD
	readToActivate := readToPrecharge + prechargeToActivate
	writeToActivate := writeToPrecharge + prechargeToActivate

	activateToActivate := s.tRC()
	activateToActivateL := s.TRRDL
	activateToActivateS := s.TRRDS
	activateToPrecharge := s.TRAS
	activateToRead := s.TRCD - s.TAL
	activateToWrite := s.TRCD - s.TAL

	if proto.isGDDR() || proto.isHBM() {
		activateToRead = s.TRCDRD
		activateToWrite = s.TRCDWR
	}

	activateToRefresh := s.tRC()

	refreshToRefresh := s.TREFI
	refreshToActivate := s.TRFC
	refreshToActivateBank := s.TRFCb

	selfRefreshEntryToExit := s.TCKESR
	selfRefreshExit := s.TXS

	if s.NumBankGroup == 1 {
		readToReadL = max(burstCycle, s.TCCDS)
		writeToReadL = writeDelay + s.TWTRS
		writeToWriteL = max(burstCycle, s.TCCDS)
		activateToActivateL = s.TRRDS
	}

	t.SameBank[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: readToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	t.SameBank[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: writeToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// READ_PRECHARGE
	t.SameBank[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: readpToAct},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: readToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: readToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: readToActivate},
	}
	t.OtherBanksInBankGroup[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	// WRITE_PRECHARGE
	t.SameBank[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: writeToActivate},
	}
	t.OtherBanksInBankGroup[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// ACTIVATE
	t.SameBank[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivate},
		{NextCmdKind: cmdKindRead, MinCycleInBetween: activateToRead},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: activateToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: activateToRead},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: activateToWrite},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: activateToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivateL},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}
	t.SameRank[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivateS},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}

	// PRECHARGE
	t.SameBank[cmdKindPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: prechargeToActivate},
	}

	if proto.isGDDR() || proto == protoLPDDR4 {
		t.OtherBanksInBankGroup[cmdKindPrecharge] = []timeTableEntry{
			{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
		t.SameRank[cmdKindPrecharge] = []timeTableEntry{
			{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
	}

	// REFRESH_BANK
	t.SameRank[cmdKindRefreshBank] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: max(refreshToActivateBank, refreshToActivate)},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: refreshToActivateBank},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: max(refreshToActivateBank, refreshToRefresh)},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: refreshToActivateBank},
	}
	t.OtherBanksInBankGroup[cmdKindRefreshBank] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: refreshToRefresh},
	}

	// REFRESH
	t.SameRank[cmdKindRefresh] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: refreshToActivate},
	}

	// SREF_ENTER
	t.SameRank[cmdKindSRefEnter] = []timeTableEntry{
		{NextCmdKind: cmdKindSRefExit, MinCycleInBetween: selfRefreshEntryToExit},
	}

	// SREF_EXIT
	t.SameRank[cmdKindSRefExit] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: selfRefreshExit},
	}

	return t
}
