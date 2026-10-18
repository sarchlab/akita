package dram

import (
	"fmt"

	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Helper: build timing + cmdCycles for any Spec (as the component does when
// it is built). NOTE: In this codebase writeDelay = tRL + burstCycle, NOT
// tWL + burstCycle. This is a known deviation from DRAMSim3 where
// writeDelay = tWL + burstCycle.
// ---------------------------------------------------------------
func buildTimingForSpec(spec Spec) (dramTiming, map[commandKind]int) {
	return generateTiming(&spec), buildCmdCycles(&spec)
}

// ---------------------------------------------------------------
// Helper: look up a timing value from a TimeTable
// ---------------------------------------------------------------
func lookupTiming(table timeTable, srcCmd, dstCmd commandKind) (int, bool) {
	if int(srcCmd) >= len(table) {
		return 0, false
	}
	for _, entry := range table[srcCmd] {
		if entry.NextCmdKind == dstCmd {
			return entry.MinCycleInBetween, true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------
// Helper: create fresh State for any Spec
// ---------------------------------------------------------------
func newStateForSpec(spec Spec) *state {
	return &state{
		BankStates: initBankStatesFlat(
			spec.NumRank, spec.NumBankGroup, spec.NumBank,
		),
	}
}

// ---------------------------------------------------------------
// Independent formula computation (DRAMSim3 / Ramulator2 style)
// ---------------------------------------------------------------
type expectedTimings struct {
	burstCycle int
	tRL        int
	tWL        int
	readDelay  int
	writeDelay int
	tRC        int

	readToReadL  int
	readToReadS  int
	readToReadO  int
	readToWrite  int
	readToWriteO int

	writeToReadL  int
	writeToReadS  int
	writeToReadO  int
	writeToWriteL int
	writeToWriteS int
	writeToWriteO int

	writeToPrecharge    int
	readToPrecharge     int
	prechargeToActivate int
	activateToRead      int
	activateToWrite     int
	activateToActivate  int
	activateToActivateL int
	activateToActivateS int
	activateToPrecharge int
}

func computeBurstCycle(spec Spec) int {
	protocol := protocol(spec.Protocol)

	burstCycle := spec.BurstLength / 2

	switch protocol {
	case protoGDDR5:
		burstCycle = spec.BurstLength / 4
	case protoGDDR5X:
		burstCycle = spec.BurstLength / 8
	case protoGDDR6:
		burstCycle = spec.BurstLength / 16
	}

	return burstCycle
}

func computeBaseTimings(spec Spec, burstCycle int) (int, int, int, int, int) {
	tRL := spec.TAL + spec.TCL
	tWL := spec.TAL + spec.TCWL
	readDelay := tRL + burstCycle
	// Known deviation: writeDelay = readDelay in this codebase (NOT tWL + burstCycle).
	// DRAMSim3 uses writeDelay = tWL + burstCycle. Document as known model gap.
	writeDelay := tRL + burstCycle
	tRC := spec.TRAS + spec.TRP

	return tRL, tWL, readDelay, writeDelay, tRC
}

func computeReadTimings(
	spec Spec, burstCycle, tRL, tWL, readDelay, writeDelay int,
) (readToReadL, readToReadS, readToReadO, readToWrite, readToWriteO int) {
	readToReadL = max(burstCycle, spec.TCCDL)
	readToReadS = max(burstCycle, spec.TCCDS)
	readToReadO = burstCycle + spec.TRTRS
	readToWrite = tRL + burstCycle - tWL + spec.TRTRS
	readToWriteO = readDelay + burstCycle + spec.TRTRS - writeDelay

	if spec.NumBankGroup == 1 {
		readToReadL = max(burstCycle, spec.TCCDS)
	}

	return
}

func computeWriteTimings(
	spec Spec, burstCycle, tWL, readDelay, writeDelay int,
) (wtrL, wtrS, wtrO, wtwL, wtwS, wtwO, wtoPre int) {
	wtrL = writeDelay + spec.TWTRL
	wtrS = writeDelay + spec.TWTRS
	wtrO = writeDelay + burstCycle + spec.TRTRS - readDelay
	wtwL = max(burstCycle, spec.TCCDL)
	wtwS = max(burstCycle, spec.TCCDS)
	wtwO = burstCycle
	wtoPre = tWL + burstCycle + spec.TWR

	if spec.NumBankGroup == 1 {
		wtrL = writeDelay + spec.TWTRS
		wtwL = max(burstCycle, spec.TCCDS)
	}

	return
}

func computeActivateTimings(spec Spec, tRC int) (
	actToRead, actToWrite, actToAct, actToActL, actToActS, actToPre int,
) {
	protocol := protocol(spec.Protocol)

	actToRead = spec.TRCD - spec.TAL
	actToWrite = spec.TRCD - spec.TAL

	if protocol.isGDDR() || protocol.isHBM() {
		actToRead = spec.TRCDRD
		actToWrite = spec.TRCDWR
	}

	actToAct = tRC
	actToActL = spec.TRRDL
	actToActS = spec.TRRDS
	actToPre = spec.TRAS

	if spec.NumBankGroup == 1 {
		actToActL = spec.TRRDS
	}

	return
}

func computeExpectedTimings(spec Spec) expectedTimings {
	burstCycle := computeBurstCycle(spec)
	tRL, tWL, readDelay, writeDelay, tRC := computeBaseTimings(spec, burstCycle)

	rrL, rrS, rrO, rw, rwO := computeReadTimings(
		spec, burstCycle, tRL, tWL, readDelay, writeDelay)
	wrL, wrS, wrO, wwL, wwS, wwO, wtoPre := computeWriteTimings(
		spec, burstCycle, tWL, readDelay, writeDelay)
	aR, aW, aA, aAL, aAS, aP := computeActivateTimings(spec, tRC)

	return expectedTimings{
		burstCycle: burstCycle, tRL: tRL, tWL: tWL,
		readDelay: readDelay, writeDelay: writeDelay, tRC: tRC,
		readToReadL: rrL, readToReadS: rrS, readToReadO: rrO,
		readToWrite: rw, readToWriteO: rwO,
		writeToReadL: wrL, writeToReadS: wrS, writeToReadO: wrO,
		writeToWriteL: wwL, writeToWriteS: wwS, writeToWriteO: wwO,
		writeToPrecharge:    wtoPre,
		readToPrecharge:     spec.TAL + spec.TRTP,
		prechargeToActivate: spec.TRP,
		activateToRead:      aR, activateToWrite: aW,
		activateToActivate: aA, activateToActivateL: aAL,
		activateToActivateS: aAS, activateToPrecharge: aP,
	}
}

// =====================================================================
// Tier 1: Timing Formula Cross-Validation
// =====================================================================
//
//nolint:funlen,gocognit // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestTimingCrossValidation(t *testing.T) {
	// -----------------------------------------------------------------
	// Tier 1 tests: verify generateTiming() matches independent formulas
	// -----------------------------------------------------------------
	t.Run("Tier 1: Timing Formula Cross-Validation", func(t *testing.T) {
		type specCase struct {
			name string
			spec Spec
		}

		specs := []specCase{
			{"DDR4-2400", DDR4Spec()},
			{"DDR5-4800", DDR5Spec()},
			{"HBM2-2Gbps", HBM2Spec()},
		}

		for _, sc := range specs {
			sc := sc // capture
			t.Run(sc.name, func(t *testing.T) {
				var (
					timing   dramTiming
					expected expectedTimings
				)

				setup1 := func(t *testing.T) {
					t.Helper()

					timing, _ = buildTimingForSpec(sc.spec)
					expected = computeExpectedTimings(sc.spec)
				}

				// --- Read → Read ---
				t.Run("should match readToRead_L (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindRead, cmdKindRead)
					require.True(t, ok, "Read→Read not found in SameBank")
					require.Equal(t, expected.readToReadL, got, fmt.Sprintf("readToReadL: got %d, want %d",
						got, expected.readToReadL))
				})

				t.Run("should match readToRead_L (OtherBanksInBankGroup)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(
						timing.OtherBanksInBankGroup,
						cmdKindRead, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.readToReadL, got)
				})

				t.Run("should match readToRead_S (SameRank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameRank,
						cmdKindRead, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.readToReadS, got)
				})

				t.Run("should match readToRead_O (OtherRanks)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.OtherRanks,
						cmdKindRead, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.readToReadO, got)
				})

				// --- Read → Write ---
				t.Run("should match readToWrite (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindRead, cmdKindWrite)
					require.True(t, ok)
					require.Equal(t, expected.readToWrite, got)
				})

				t.Run("should match readToWrite_O (OtherRanks)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.OtherRanks,
						cmdKindRead, cmdKindWritePrecharge)
					require.True(t, ok)
					require.Equal(t, expected.readToWriteO, got)
				})

				// --- Write → Read ---
				t.Run("should match writeToRead_L (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindWrite, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.writeToReadL, got)
				})

				t.Run("should match writeToRead_S (SameRank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameRank,
						cmdKindWrite, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.writeToReadS, got)
				})

				t.Run("should match writeToRead_O (OtherRanks)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.OtherRanks,
						cmdKindWrite, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.writeToReadO, got)
				})

				// --- Write → Write ---
				t.Run("should match writeToWrite_L (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindWrite, cmdKindWrite)
					require.True(t, ok)
					require.Equal(t, expected.writeToWriteL, got)
				})

				t.Run("should match writeToWrite_S (SameRank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameRank,
						cmdKindWrite, cmdKindWrite)
					require.True(t, ok)
					require.Equal(t, expected.writeToWriteS, got)
				})

				t.Run("should match writeToWrite_O (OtherRanks)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.OtherRanks,
						cmdKindWrite, cmdKindWrite)
					require.True(t, ok)
					require.Equal(t, expected.writeToWriteO, got)
				})

				// --- Write → Precharge ---
				t.Run("should match writeToPrecharge (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindWrite, cmdKindPrecharge)
					require.True(t, ok)
					require.Equal(t, expected.writeToPrecharge, got)
				})

				// --- Read → Precharge ---
				t.Run("should match readToPrecharge (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindRead, cmdKindPrecharge)
					require.True(t, ok)
					require.Equal(t, expected.readToPrecharge, got)
				})

				// --- Precharge → Activate ---
				t.Run("should match prechargeToActivate (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindPrecharge, cmdKindActivate)
					require.True(t, ok)
					require.Equal(t, expected.prechargeToActivate, got)
				})

				// --- Activate → Read / Write ---
				t.Run("should match activateToRead (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindActivate, cmdKindRead)
					require.True(t, ok)
					require.Equal(t, expected.activateToRead, got)
				})

				t.Run("should match activateToWrite (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindActivate, cmdKindWrite)
					require.True(t, ok)
					require.Equal(t, expected.activateToWrite, got)
				})

				// --- Activate → Activate ---
				t.Run("should match activateToActivate (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindActivate, cmdKindActivate)
					require.True(t, ok)
					require.Equal(t, expected.activateToActivate, got)
				})

				t.Run("should match activateToActivate_L (OtherBanksInBankGroup)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(
						timing.OtherBanksInBankGroup,
						cmdKindActivate, cmdKindActivate)
					require.True(t, ok)
					require.Equal(t, expected.activateToActivateL, got)
				})

				t.Run("should match activateToActivate_S (SameRank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameRank,
						cmdKindActivate, cmdKindActivate)
					require.True(t, ok)
					require.Equal(t, expected.activateToActivateS, got)
				})

				// --- Activate → Precharge ---
				t.Run("should match activateToPrecharge (SameBank)", func(t *testing.T) {
					setup1(t)

					got, ok := lookupTiming(timing.SameBank,
						cmdKindActivate, cmdKindPrecharge)
					require.True(t, ok)
					require.Equal(t, expected.activateToPrecharge, got)
				})
			})
		}
	})

	// =================================================================
	// Tier 2: Single-Request Latency Tests (DDR4)
	// =================================================================
	t.Run("Tier 2: Single-Request Latency", func(t *testing.T) {
		var (
			timing    dramTiming
			cmdCycles map[commandKind]int
			st        *state
			spec      Spec
		)

		setup2 := func(t *testing.T) {
			t.Helper()

			spec = DDR4Spec()
			timing, cmdCycles = buildTimingForSpec(spec)
			st = newStateForSpec(spec)
		}

		t.Run("closed-bank read: total cycles = tRCD + readDelay", func(t *testing.T) {
			setup2(t)

			// A read to a closed bank requires ACT + wait tRCD + READ.
			bs := findBankState(&st.BankStates, 0, 0, 0)
			require.Equal(t, bankStateClosed, bankStateKind(bs.State))

			// Determine required command: should be Activate.
			readCmd := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			require.Equal(t, cmdKindActivate, reqKind)

			// Issue Activate.
			actCmd := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			startCommand(cmdCycles, st, bs, actCmd)
			updateTiming(timing, st, actCmd)

			// Activate→Read gap = tRCD - tAL = 16
			require.Equal(t, spec.TRCD-spec.TAL, bs.CyclesToCmdAvailable[cmdKindRead])

			// Tick until the Activate→Read gap drains.
			tRCDcycles := spec.TRCD - spec.TAL
			for range tRCDcycles {
				tickBanks(st)
			}
			require.Equal(t, 0, bs.CyclesToCmdAvailable[cmdKindRead])

			// Now issue Read.
			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindRead, commandKind(ready.Kind))

			startCommand(cmdCycles, st, bs, ready)
			updateTiming(timing, st, ready)

			// The read's data returns readDelay cycles after issue. With
			// TickCount still at 0 here, the scheduled completion tick equals
			// readDelay.
			exp := computeExpectedTimings(spec)
			require.NotEmpty(t, st.PendingCompletions)
			last := st.PendingCompletions[len(st.PendingCompletions)-1]
			require.Equal(t, uint64(exp.readDelay), last.CompletionTick)

			// Total cycles = tRCD + readDelay
			totalCycles := tRCDcycles + exp.readDelay
			require.Equal(t, spec.TRCD-spec.TAL+exp.tRL+exp.burstCycle, totalCycles)
		})

		t.Run("row-buffer-hit read: no ACT needed", func(t *testing.T) {
			setup2(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			// Open bank to row 42.
			actCmd := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 42},
			}
			startCommand(cmdCycles, st, bs, actCmd)
			updateTiming(timing, st, actCmd)
			for range spec.TRCD {
				tickBanks(st)
			}

			// Read same row → should immediately return Read.
			readCmd := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 42},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			require.Equal(t, cmdKindRead, reqKind)

			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindRead, commandKind(ready.Kind))
		})

		t.Run("row-conflict read: PRE + tRP + ACT + tRCD + READ", func(t *testing.T) {
			setup2(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			// Open bank to row 100.
			actCmd := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 100},
			}
			startCommand(cmdCycles, st, bs, actCmd)
			updateTiming(timing, st, actCmd)
			for range spec.TRCD {
				tickBanks(st)
			}

			// Request read for different row → row conflict → Precharge first.
			readCmd := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 200},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			require.Equal(t, cmdKindPrecharge, reqKind)

			// Wait for activateToPrecharge timing (tRAS).
			// After tRCD ticks the ACT→PRE constraint has partially drained.
			// The remaining is tRAS - tRCD.
			preKey := cmdKindPrecharge
			remaining := bs.CyclesToCmdAvailable[preKey]
			for range remaining {
				tickBanks(st)
			}

			// Issue Precharge.
			preCmd := &commandState{
				Kind:     int(cmdKindPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 100},
			}
			startCommand(cmdCycles, st, bs, preCmd)
			updateTiming(timing, st, preCmd)
			require.Equal(t, bankStateClosed, bankStateKind(bs.State))

			// Wait tRP for Precharge to complete.
			for range spec.TRP {
				tickBanks(st)
			}

			// Now Activate should be ready.
			reqKind2 := getRequiredCommandKind(bs, readCmd)
			require.Equal(t, cmdKindActivate, reqKind2)

			actCmd2 := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, actCmd2)
			require.Equal(t, cmdKindActivate, commandKind(actCmd2.Kind))
		})

		t.Run("write-then-read same bank: writeToReadL gap enforced", func(t *testing.T) {
			setup2(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			// Open bank.
			actCmd := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			startCommand(cmdCycles, st, bs, actCmd)
			updateTiming(timing, st, actCmd)
			for range spec.TRCD {
				tickBanks(st)
			}

			// Issue Write.
			writeCmd := &commandState{
				Kind:     int(cmdKindWrite),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			startCommand(cmdCycles, st, bs, writeCmd)
			updateTiming(timing, st, writeCmd)

			// Check Write→Read constraint on same bank.
			readKey := cmdKindRead
			exp := computeExpectedTimings(spec)
			require.Equal(t, exp.writeToReadL, bs.CyclesToCmdAvailable[readKey], "writeToReadL constraint not set correctly")

			// Read should NOT be ready immediately.
			readCmd := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			// The bank is busy with the write command, so wait for it.
			for range exp.readDelay {
				tickBanks(st)
			}

			// After readDelay ticks, the write command should be done,
			// but we may still need more ticks for writeToReadL.
			// Check remaining constraint.
			remainingWTR := bs.CyclesToCmdAvailable[readKey]
			if remainingWTR > 0 {
				for range remainingWTR {
					tickBanks(st)
				}
			}

			// Now Read should be ready.
			require.Equal(t, 0, bs.CyclesToCmdAvailable[readKey])
			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindRead, commandKind(ready.Kind))
		})
	})

	// =================================================================
	// Tier 3: Multi-Request Behavioral Tests (DDR4)
	// =================================================================
	t.Run("Tier 3: Multi-Request Behavioral", func(t *testing.T) {
		var (
			timing    dramTiming
			cmdCycles map[commandKind]int
			st        *state
			spec      Spec
		)

		setup3 := func(t *testing.T) {
			t.Helper()

			spec = DDR4Spec()
			timing, cmdCycles = buildTimingForSpec(spec)
			st = newStateForSpec(spec)
		}

		t.Run("sequential reads to same row: only first ACT, subsequent pay CCD", func(t *testing.T) {
			setup3(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			// Open the bank.
			actCmd := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			startCommand(cmdCycles, st, bs, actCmd)
			updateTiming(timing, st, actCmd)
			for range spec.TRCD {
				tickBanks(st)
			}

			exp := computeExpectedTimings(spec)

			// Issue first read.
			read1 := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			startCommand(cmdCycles, st, bs, read1)
			updateTiming(timing, st, read1)

			// Same bank Read→Read = tCCDL (=6 for DDR4)
			readKey := cmdKindRead
			require.Equal(t, exp.readToReadL, bs.CyclesToCmdAvailable[readKey])

			// Tick until read completes and CCD constraint drains.
			drainCycles := max(exp.readDelay, exp.readToReadL)
			for range drainCycles {
				tickBanks(st)
			}

			// Second read: should be ready (no new ACT needed).
			read2 := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			reqKind := getRequiredCommandKind(bs, read2)
			require.Equal(t, cmdKindRead, reqKind, "second read should not need ACT (row buffer hit)")

			ready := getReadyCommand(&spec, st, bs, read2)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindRead, commandKind(ready.Kind))
		})

		t.Run("parallel bank reads: bounded by tRRD", func(t *testing.T) {
			setup3(t)

			// Open two banks in different bank groups and verify
			// the Activate→Activate constraint between them.
			bs0 := findBankState(&st.BankStates, 0, 0, 0)
			bs1 := findBankState(&st.BankStates, 0, 1, 0)

			// Activate bank (0,0,0).
			act0 := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			startCommand(cmdCycles, st, bs0, act0)
			updateTiming(timing, st, act0)

			// Check that bank (0,1,0) has tRRDS constraint.
			actKey := cmdKindActivate
			constraint := bs1.CyclesToCmdAvailable[actKey]
			exp := computeExpectedTimings(spec)
			require.Equal(t, exp.activateToActivateS, constraint, fmt.Sprintf("different bank group ACT→ACT should be tRRDS=%d",
				exp.activateToActivateS))

			// Tick until constraint drains.
			for range constraint {
				tickBanks(st)
			}

			// Now Activate on bank (0,1,0) should be ready.
			act1Cmd := &commandState{
				Kind:     int(cmdKindRead),
				Location: location{Rank: 0, BankGroup: 1, Bank: 0, Row: 2},
			}
			ready := getReadyCommand(&spec, st, bs1, act1Cmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindActivate, commandKind(ready.Kind))
		})

		t.Run("same bank-group reads: bounded by tRRDL", func(t *testing.T) {
			setup3(t)

			// Activate on bank (0,0,0), check constraint on bank (0,0,1).
			bs0 := findBankState(&st.BankStates, 0, 0, 0)
			bs1 := findBankState(&st.BankStates, 0, 0, 1)

			act0 := &commandState{
				Kind:     int(cmdKindActivate),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 5},
			}
			startCommand(cmdCycles, st, bs0, act0)
			updateTiming(timing, st, act0)

			actKey := cmdKindActivate
			constraint := bs1.CyclesToCmdAvailable[actKey]
			exp := computeExpectedTimings(spec)
			require.Equal(t, exp.activateToActivateL, constraint, fmt.Sprintf("same bank group ACT→ACT should be tRRDL=%d",
				exp.activateToActivateL))
		})

		t.Run("5+ activates: tFAW enforcement", func(t *testing.T) {
			setup3(t)

			// Issue 4 activates on different banks within a short window.
			spec.TFAW = 28

			// Rebuild timing with updated spec.
			timing, cmdCycles = buildTimingForSpec(spec)
			st = newStateForSpec(spec)

			for i := range 4 {
				st.TickCount = uint64(i * 2)
				bs := findBankState(&st.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(cmdKindActivate),
					Location: location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(300 + i),
					},
				}
				startCommand(cmdCycles, st, bs, cmd)
				updateTiming(timing, st, cmd)
			}

			// 5th activate within tFAW window → should be blocked.
			st.TickCount = 7 // oldest was at tick 0, 7 < 28
			bs := findBankState(&st.BankStates, 0, 0, 1)
			bs.State = int(bankStateClosed)
			bs.CyclesToCmdAvailable = [numCmdKind]int{}

			readCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank: 0, BankGroup: 0, Bank: 1, Row: 999,
				},
			}
			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.Nil(t, ready, "5th activate within tFAW window should be blocked")

			// After tFAW passes, should be allowed.
			st.TickCount = 28
			ready = getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready, "activate should be allowed after tFAW passes")
			require.Equal(t, cmdKindActivate, commandKind(ready.Kind))
		})
	})

	// =================================================================
	// Tier 4: Bandwidth Sanity Checks (analytical)
	// =================================================================
	t.Run("Tier 4: Bandwidth Sanity", func(t *testing.T) {
		t.Run("DDR4-2400 streaming reads: 40-90% of 19.2 GB/s peak", func(t *testing.T) {
			spec := DDR4Spec()
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// DDR4-2400: 1200 MHz * 64 bits * 2 (DDR) / 8 = 19.2 GB/s
			peakBWGBs := float64(1200e6) * float64(64) * 2.0 / 8.0 / 1e9
			require.InDelta(t, 19.2, peakBWGBs, 0.01)

			// Estimated achievable bandwidth from timing:
			// For streaming reads to the same row (row-buffer hits),
			// the inter-read gap is readToReadL = max(burstCycle, tCCDL).
			// Each read transfers burstLength * busWidth / 8 bytes.
			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL) // tCCDL for back-to-back
			freq := 1200e6                            // Hz
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			ratio := achievableBW / peakBWGBs
			require.True(t, (ratio) >= (0.40), fmt.Sprintf("DDR4 achievable BW ratio %.2f < 0.40", ratio))
			require.True(t, (ratio) <= (0.90), fmt.Sprintf("DDR4 achievable BW ratio %.2f > 0.90", ratio))
		})

		t.Run("HBM2-2Gbps streaming reads: 40-90% of 32 GB/s peak", func(t *testing.T) {
			spec := HBM2Spec()
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// HBM2: 1000 MHz * 128 bits * 2 (DDR) / 8 = 32 GB/s
			peakBWGBs := float64(1000e6) * float64(128) * 2.0 / 8.0 / 1e9
			require.InDelta(t, 32.0, peakBWGBs, 0.01)

			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL)
			freq := 1000e6
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			ratio := achievableBW / peakBWGBs
			require.True(t, (ratio) >= (0.40), fmt.Sprintf("HBM2 achievable BW ratio %.2f < 0.40", ratio))
			require.True(t, (ratio) <= (0.90), fmt.Sprintf("HBM2 achievable BW ratio %.2f > 0.90", ratio))
		})

		t.Run("DDR5-4800 streaming reads: 40-100% of 19.2 GB/s peak", func(t *testing.T) {
			spec := DDR5Spec()
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// DDR5-4800: 2400 MHz * 32 bits * 2 (DDR) / 8 = 19.2 GB/s
			peakBWGBs := float64(2400e6) * float64(32) * 2.0 / 8.0 / 1e9
			require.InDelta(t, 19.2, peakBWGBs, 0.01)

			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL)
			freq := 2400e6
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			// DDR5 has tCCDL == burstCycle == 8, so row-buffer-hit reads can
			// theoretically achieve 100% of peak. Use 40-100% range.
			ratio := achievableBW / peakBWGBs
			require.True(t, (ratio) >= (0.40), fmt.Sprintf("DDR5 achievable BW ratio %.2f < 0.40", ratio))
			require.True(t, (ratio) <= (1.00), fmt.Sprintf("DDR5 achievable BW ratio %.2f > 1.00", ratio))
		})
	})
}
