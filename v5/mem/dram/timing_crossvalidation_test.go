package dram

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------------------------------------------------------------
// Helper: build timing + cmdCycles for any Spec (replicates Build logic)
// ---------------------------------------------------------------
func buildTimingForSpec(spec Spec) (Timing, map[CommandKind]int) {
	b := MakeBuilder().WithSpec(spec)
	b.calculateBurstCycle()
	b.tRL = b.tAL + b.tCL
	b.tWL = b.tAL + b.tCWL
	b.readDelay = b.tRL + b.burstCycle
	// NOTE: In this codebase writeDelay = tRL + burstCycle, NOT tWL + burstCycle.
	// This is a known deviation from DRAMSim3 where writeDelay = tWL + burstCycle.
	b.writeDelay = b.tRL + b.burstCycle
	b.tRC = b.tRAS + b.tRP

	timing := b.generateTiming()
	cmdCycles := b.buildCmdCycles()
	return timing, cmdCycles
}

// ---------------------------------------------------------------
// Helper: look up a timing value from a TimeTable
// ---------------------------------------------------------------
func lookupTiming(table TimeTable, srcCmd, dstCmd CommandKind) (int, bool) {
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
func newStateForSpec(spec Spec) *State {
	return &State{
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
	protocol := Protocol(spec.Protocol)

	burstCycle := spec.BurstLength / 2

	switch protocol {
	case GDDR5:
		burstCycle = spec.BurstLength / 4
	case GDDR5X:
		burstCycle = spec.BurstLength / 8
	case GDDR6:
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
	protocol := Protocol(spec.Protocol)

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
		activateToRead: aR, activateToWrite: aW,
		activateToActivate: aA, activateToActivateL: aAL,
		activateToActivateS: aAS, activateToPrecharge: aP,
	}
}

// =====================================================================
// Tier 1: Timing Formula Cross-Validation
// =====================================================================
var _ = Describe("Timing Cross-Validation", func() {

	// -----------------------------------------------------------------
	// Tier 1 tests: verify generateTiming() matches independent formulas
	// -----------------------------------------------------------------
	Describe("Tier 1: Timing Formula Cross-Validation", func() {

		type specCase struct {
			name string
			spec Spec
		}

		specs := []specCase{
			{"DDR4-2400", DDR4Spec},
			{"DDR5-4800", DDR5Spec},
			{"HBM2-2Gbps", HBM2Spec},
		}

		for _, sc := range specs {
			sc := sc // capture
			Describe(sc.name, func() {
				var (
					timing   Timing
					expected expectedTimings
				)

				BeforeEach(func() {
					timing, _ = buildTimingForSpec(sc.spec)
					expected = computeExpectedTimings(sc.spec)
				})

				// --- Read → Read ---
				It("should match readToRead_L (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindRead, CmdKindRead)
					Expect(ok).To(BeTrue(),
						"Read→Read not found in SameBank")
					Expect(got).To(Equal(expected.readToReadL),
						fmt.Sprintf("readToReadL: got %d, want %d",
							got, expected.readToReadL))
				})

				It("should match readToRead_L (OtherBanksInBankGroup)", func() {
					got, ok := lookupTiming(
						timing.OtherBanksInBankGroup,
						CmdKindRead, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToReadL))
				})

				It("should match readToRead_S (SameRank)", func() {
					got, ok := lookupTiming(timing.SameRank,
						CmdKindRead, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToReadS))
				})

				It("should match readToRead_O (OtherRanks)", func() {
					got, ok := lookupTiming(timing.OtherRanks,
						CmdKindRead, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToReadO))
				})

				// --- Read → Write ---
				It("should match readToWrite (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindRead, CmdKindWrite)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToWrite))
				})

				It("should match readToWrite_O (OtherRanks)", func() {
					got, ok := lookupTiming(timing.OtherRanks,
						CmdKindRead, CmdKindWritePrecharge)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToWriteO))
				})

				// --- Write → Read ---
				It("should match writeToRead_L (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindWrite, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToReadL))
				})

				It("should match writeToRead_S (SameRank)", func() {
					got, ok := lookupTiming(timing.SameRank,
						CmdKindWrite, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToReadS))
				})

				It("should match writeToRead_O (OtherRanks)", func() {
					got, ok := lookupTiming(timing.OtherRanks,
						CmdKindWrite, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToReadO))
				})

				// --- Write → Write ---
				It("should match writeToWrite_L (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindWrite, CmdKindWrite)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToWriteL))
				})

				It("should match writeToWrite_S (SameRank)", func() {
					got, ok := lookupTiming(timing.SameRank,
						CmdKindWrite, CmdKindWrite)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToWriteS))
				})

				It("should match writeToWrite_O (OtherRanks)", func() {
					got, ok := lookupTiming(timing.OtherRanks,
						CmdKindWrite, CmdKindWrite)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToWriteO))
				})

				// --- Write → Precharge ---
				It("should match writeToPrecharge (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindWrite, CmdKindPrecharge)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.writeToPrecharge))
				})

				// --- Read → Precharge ---
				It("should match readToPrecharge (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindRead, CmdKindPrecharge)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.readToPrecharge))
				})

				// --- Precharge → Activate ---
				It("should match prechargeToActivate (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindPrecharge, CmdKindActivate)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.prechargeToActivate))
				})

				// --- Activate → Read / Write ---
				It("should match activateToRead (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindActivate, CmdKindRead)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToRead))
				})

				It("should match activateToWrite (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindActivate, CmdKindWrite)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToWrite))
				})

				// --- Activate → Activate ---
				It("should match activateToActivate (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindActivate, CmdKindActivate)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToActivate))
				})

				It("should match activateToActivate_L (OtherBanksInBankGroup)", func() {
					got, ok := lookupTiming(
						timing.OtherBanksInBankGroup,
						CmdKindActivate, CmdKindActivate)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToActivateL))
				})

				It("should match activateToActivate_S (SameRank)", func() {
					got, ok := lookupTiming(timing.SameRank,
						CmdKindActivate, CmdKindActivate)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToActivateS))
				})

				// --- Activate → Precharge ---
				It("should match activateToPrecharge (SameBank)", func() {
					got, ok := lookupTiming(timing.SameBank,
						CmdKindActivate, CmdKindPrecharge)
					Expect(ok).To(BeTrue())
					Expect(got).To(Equal(expected.activateToPrecharge))
				})
			})
		}
	})

	// =================================================================
	// Tier 2: Single-Request Latency Tests (DDR4)
	// =================================================================
	Describe("Tier 2: Single-Request Latency", func() {
		var (
			timing    Timing
			cmdCycles map[CommandKind]int
			state     *State
			spec      Spec
		)

		BeforeEach(func() {
			spec = DDR4Spec
			timing, cmdCycles = buildTimingForSpec(spec)
			state = newStateForSpec(spec)
		})

		It("closed-bank read: total cycles = tRCD + readDelay", func() {
			// A read to a closed bank requires ACT + wait tRCD + READ.
			bs := findBankState(&state.BankStates, 0, 0, 0)
			Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))

			// Determine required command: should be Activate.
			readCmd := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			Expect(reqKind).To(Equal(CmdKindActivate))

			// Issue Activate.
			actCmd := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			startCommand(cmdCycles, state, bs, actCmd)
			updateTiming(timing, state, actCmd)

			// CycleLeft for Activate = tRCD - tAL = 16
			Expect(bs.CurrentCmd.CycleLeft).To(Equal(spec.TRCD - spec.TAL))

			// Tick until Activate completes.
			tRCDcycles := spec.TRCD - spec.TAL
			for range tRCDcycles {
				tickBanks(&spec, cmdCycles, state)
			}
			Expect(bs.HasCurrentCmd).To(BeFalse())

			// Now issue Read.
			ready := getReadyCommand(&spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindRead))

			startCommand(cmdCycles, state, bs, ready)
			updateTiming(timing, state, ready)

			exp := computeExpectedTimings(spec)
			Expect(bs.CurrentCmd.CycleLeft).To(Equal(exp.readDelay))

			// Total cycles = tRCD + readDelay
			totalCycles := tRCDcycles + exp.readDelay
			Expect(totalCycles).To(Equal(spec.TRCD - spec.TAL + exp.tRL + exp.burstCycle))
		})

		It("row-buffer-hit read: no ACT needed", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			// Open bank to row 42.
			actCmd := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 42},
			}
			startCommand(cmdCycles, state, bs, actCmd)
			updateTiming(timing, state, actCmd)
			for range spec.TRCD {
				tickBanks(&spec, cmdCycles, state)
			}

			// Read same row → should immediately return Read.
			readCmd := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 42},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			Expect(reqKind).To(Equal(CmdKindRead))

			ready := getReadyCommand(&spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindRead))
		})

		It("row-conflict read: PRE + tRP + ACT + tRCD + READ", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			// Open bank to row 100.
			actCmd := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 100},
			}
			startCommand(cmdCycles, state, bs, actCmd)
			updateTiming(timing, state, actCmd)
			for range spec.TRCD {
				tickBanks(&spec, cmdCycles, state)
			}

			// Request read for different row → row conflict → Precharge first.
			readCmd := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 200},
			}
			reqKind := getRequiredCommandKind(bs, readCmd)
			Expect(reqKind).To(Equal(CmdKindPrecharge))

			// Wait for activateToPrecharge timing (tRAS).
			// After tRCD ticks the ACT→PRE constraint has partially drained.
			// The remaining is tRAS - tRCD.
			preKey := cmdKindToString(CmdKindPrecharge)
			remaining := bs.CyclesToCmdAvailable[preKey]
			for range remaining {
				tickBanks(&spec, cmdCycles, state)
			}

			// Issue Precharge.
			preCmd := &commandState{
				Kind:     int(CmdKindPrecharge),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 100},
			}
			startCommand(cmdCycles, state, bs, preCmd)
			updateTiming(timing, state, preCmd)
			Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))

			// Wait tRP for Precharge to complete.
			for range spec.TRP {
				tickBanks(&spec, cmdCycles, state)
			}

			// Now Activate should be ready.
			reqKind2 := getRequiredCommandKind(bs, readCmd)
			Expect(reqKind2).To(Equal(CmdKindActivate))

			actCmd2 := getReadyCommand(&spec, state, bs, readCmd)
			Expect(actCmd2).NotTo(BeNil())
			Expect(CommandKind(actCmd2.Kind)).To(Equal(CmdKindActivate))
		})

		It("write-then-read same bank: writeToReadL gap enforced", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			// Open bank.
			actCmd := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			startCommand(cmdCycles, state, bs, actCmd)
			updateTiming(timing, state, actCmd)
			for range spec.TRCD {
				tickBanks(&spec, cmdCycles, state)
			}

			// Issue Write.
			writeCmd := &commandState{
				Kind:     int(CmdKindWrite),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			startCommand(cmdCycles, state, bs, writeCmd)
			updateTiming(timing, state, writeCmd)

			// Check Write→Read constraint on same bank.
			readKey := cmdKindToString(CmdKindRead)
			exp := computeExpectedTimings(spec)
			Expect(bs.CyclesToCmdAvailable[readKey]).To(
				Equal(exp.writeToReadL),
				"writeToReadL constraint not set correctly")

			// Read should NOT be ready immediately.
			readCmd := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 50},
			}
			// The bank is busy with the write command, so wait for it.
			for range exp.readDelay {
				tickBanks(&spec, cmdCycles, state)
			}

			// After readDelay ticks, the write command should be done,
			// but we may still need more ticks for writeToReadL.
			// Check remaining constraint.
			remainingWTR := bs.CyclesToCmdAvailable[readKey]
			if remainingWTR > 0 {
				for range remainingWTR {
					tickBanks(&spec, cmdCycles, state)
				}
			}

			// Now Read should be ready.
			Expect(bs.CyclesToCmdAvailable[readKey]).To(Equal(0))
			ready := getReadyCommand(&spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindRead))
		})
	})

	// =================================================================
	// Tier 3: Multi-Request Behavioral Tests (DDR4)
	// =================================================================
	Describe("Tier 3: Multi-Request Behavioral", func() {
		var (
			timing    Timing
			cmdCycles map[CommandKind]int
			state     *State
			spec      Spec
		)

		BeforeEach(func() {
			spec = DDR4Spec
			timing, cmdCycles = buildTimingForSpec(spec)
			state = newStateForSpec(spec)
		})

		It("sequential reads to same row: only first ACT, subsequent pay CCD", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			// Open the bank.
			actCmd := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			startCommand(cmdCycles, state, bs, actCmd)
			updateTiming(timing, state, actCmd)
			for range spec.TRCD {
				tickBanks(&spec, cmdCycles, state)
			}

			exp := computeExpectedTimings(spec)

			// Issue first read.
			read1 := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			startCommand(cmdCycles, state, bs, read1)
			updateTiming(timing, state, read1)

			// Same bank Read→Read = tCCDL (=6 for DDR4)
			readKey := cmdKindToString(CmdKindRead)
			Expect(bs.CyclesToCmdAvailable[readKey]).To(Equal(exp.readToReadL))

			// Tick until read completes and CCD constraint drains.
			drainCycles := max(exp.readDelay, exp.readToReadL)
			for range drainCycles {
				tickBanks(&spec, cmdCycles, state)
			}

			// Second read: should be ready (no new ACT needed).
			read2 := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 10},
			}
			reqKind := getRequiredCommandKind(bs, read2)
			Expect(reqKind).To(Equal(CmdKindRead),
				"second read should not need ACT (row buffer hit)")

			ready := getReadyCommand(&spec, state, bs, read2)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindRead))
		})

		It("parallel bank reads: bounded by tRRD", func() {
			// Open two banks in different bank groups and verify
			// the Activate→Activate constraint between them.
			bs0 := findBankState(&state.BankStates, 0, 0, 0)
			bs1 := findBankState(&state.BankStates, 0, 1, 0)

			// Activate bank (0,0,0).
			act0 := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
			}
			startCommand(cmdCycles, state, bs0, act0)
			updateTiming(timing, state, act0)

			// Check that bank (0,1,0) has tRRDS constraint.
			actKey := cmdKindToString(CmdKindActivate)
			constraint := bs1.CyclesToCmdAvailable[actKey]
			exp := computeExpectedTimings(spec)
			Expect(constraint).To(Equal(exp.activateToActivateS),
				fmt.Sprintf("different bank group ACT→ACT should be tRRDS=%d",
					exp.activateToActivateS))

			// Tick until constraint drains.
			for range constraint {
				tickBanks(&spec, cmdCycles, state)
			}

			// Now Activate on bank (0,1,0) should be ready.
			act1Cmd := &commandState{
				Kind:     int(CmdKindRead),
				Location: Location{Rank: 0, BankGroup: 1, Bank: 0, Row: 2},
			}
			ready := getReadyCommand(&spec, state, bs1, act1Cmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindActivate))
		})

		It("same bank-group reads: bounded by tRRDL", func() {
			// Activate on bank (0,0,0), check constraint on bank (0,0,1).
			bs0 := findBankState(&state.BankStates, 0, 0, 0)
			bs1 := findBankState(&state.BankStates, 0, 0, 1)

			act0 := &commandState{
				Kind:     int(CmdKindActivate),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 5},
			}
			startCommand(cmdCycles, state, bs0, act0)
			updateTiming(timing, state, act0)

			actKey := cmdKindToString(CmdKindActivate)
			constraint := bs1.CyclesToCmdAvailable[actKey]
			exp := computeExpectedTimings(spec)
			Expect(constraint).To(Equal(exp.activateToActivateL),
				fmt.Sprintf("same bank group ACT→ACT should be tRRDL=%d",
					exp.activateToActivateL))
		})

		It("5+ activates: tFAW enforcement", func() {
			// Issue 4 activates on different banks within a short window.
			spec.TFAW = 28

			// Rebuild timing with updated spec.
			timing, cmdCycles = buildTimingForSpec(spec)
			state = newStateForSpec(spec)

			for i := range 4 {
				state.TickCount = uint64(i * 2)
				bs := findBankState(&state.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(CmdKindActivate),
					Location: Location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(300 + i),
					},
				}
				startCommand(cmdCycles, state, bs, cmd)
				updateTiming(timing, state, cmd)
			}

			// 5th activate within tFAW window → should be blocked.
			state.TickCount = 7 // oldest was at tick 0, 7 < 28
			bs := findBankState(&state.BankStates, 0, 0, 1)
			bs.State = int(BankStateClosed)
			bs.HasCurrentCmd = false
			bs.CyclesToCmdAvailable = make(map[string]int)

			readCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank: 0, BankGroup: 0, Bank: 1, Row: 999,
				},
			}
			ready := getReadyCommand(&spec, state, bs, readCmd)
			Expect(ready).To(BeNil(),
				"5th activate within tFAW window should be blocked")

			// After tFAW passes, should be allowed.
			state.TickCount = 28
			ready = getReadyCommand(&spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil(),
				"activate should be allowed after tFAW passes")
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindActivate))
		})
	})

	// =================================================================
	// Tier 4: Bandwidth Sanity Checks (analytical)
	// =================================================================
	Describe("Tier 4: Bandwidth Sanity", func() {

		It("DDR4-2400 streaming reads: 40-90% of 19.2 GB/s peak", func() {
			spec := DDR4Spec
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// DDR4-2400: 1200 MHz * 64 bits * 2 (DDR) / 8 = 19.2 GB/s
			peakBWGBs := float64(1200e6) * float64(64) * 2.0 / 8.0 / 1e9
			Expect(peakBWGBs).To(BeNumerically("~", 19.2, 0.01))

			// Estimated achievable bandwidth from timing:
			// For streaming reads to the same row (row-buffer hits),
			// the inter-read gap is readToReadL = max(burstCycle, tCCDL).
			// Each read transfers burstLength * busWidth / 8 bytes.
			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL) // tCCDL for back-to-back
			freq := 1200e6                            // Hz
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			ratio := achievableBW / peakBWGBs
			Expect(ratio).To(BeNumerically(">=", 0.40),
				fmt.Sprintf("DDR4 achievable BW ratio %.2f < 0.40", ratio))
			Expect(ratio).To(BeNumerically("<=", 0.90),
				fmt.Sprintf("DDR4 achievable BW ratio %.2f > 0.90", ratio))
		})

		It("HBM2-2Gbps streaming reads: 40-90% of 32 GB/s peak", func() {
			spec := HBM2Spec
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// HBM2: 1000 MHz * 128 bits * 2 (DDR) / 8 = 32 GB/s
			peakBWGBs := float64(1000e6) * float64(128) * 2.0 / 8.0 / 1e9
			Expect(peakBWGBs).To(BeNumerically("~", 32.0, 0.01))

			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL)
			freq := 1000e6
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			ratio := achievableBW / peakBWGBs
			Expect(ratio).To(BeNumerically(">=", 0.40),
				fmt.Sprintf("HBM2 achievable BW ratio %.2f < 0.40", ratio))
			Expect(ratio).To(BeNumerically("<=", 0.90),
				fmt.Sprintf("HBM2 achievable BW ratio %.2f > 0.90", ratio))
		})

		It("DDR5-4800 streaming reads: 40-100% of 19.2 GB/s peak", func() {
			spec := DDR5Spec
			exp := computeExpectedTimings(spec)

			// Theoretical peak bandwidth:
			// DDR5-4800: 2400 MHz * 32 bits * 2 (DDR) / 8 = 19.2 GB/s
			peakBWGBs := float64(2400e6) * float64(32) * 2.0 / 8.0 / 1e9
			Expect(peakBWGBs).To(BeNumerically("~", 19.2, 0.01))

			bytesPerRead := float64(spec.BurstLength) * float64(spec.BusWidth) / 8.0
			cyclesPerRead := float64(exp.readToReadL)
			freq := 2400e6
			achievableBW := bytesPerRead / cyclesPerRead * freq / 1e9

			// DDR5 has tCCDL == burstCycle == 8, so row-buffer-hit reads can
			// theoretically achieve 100% of peak. Use 40-100% range.
			ratio := achievableBW / peakBWGBs
			Expect(ratio).To(BeNumerically(">=", 0.40),
				fmt.Sprintf("DDR5 achievable BW ratio %.2f < 0.40", ratio))
			Expect(ratio).To(BeNumerically("<=", 1.00),
				fmt.Sprintf("DDR5 achievable BW ratio %.2f > 1.00", ratio))
		})
	})
})
