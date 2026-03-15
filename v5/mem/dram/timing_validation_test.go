package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildDDR4TimingAndCycles replicates the builder's logic to generate
// Timing and cmdCycles for the DDR4 spec without needing to call private
// builder methods or construct a full Component.
func buildDDR4TimingAndCycles() (Timing, map[CommandKind]int) {
	b := MakeBuilder().WithSpec(DDR4Spec)

	// Replicate the computed fields that Build() would calculate.
	// DDR4 burstCycle = burstLength / 2 = 8 / 2 = 4
	b.burstCycle = b.burstLength / 2
	b.tRL = b.tAL + b.tCL     // 0 + 16 = 16
	b.tWL = b.tAL + b.tCWL    // 0 + 12 = 12
	b.readDelay = b.tRL + b.burstCycle  // 16 + 4 = 20
	b.writeDelay = b.tRL + b.burstCycle // 16 + 4 = 20
	b.tRC = b.tRAS + b.tRP    // 39 + 16 = 55

	timing := b.generateTiming()
	cmdCycles := b.buildCmdCycles()

	return timing, cmdCycles
}

// newDDR4State creates a fresh State with DDR4 bank layout (1 rank, 4 bank groups, 4 banks).
func newDDR4State() *State {
	state := &State{
		BankStates: initBankStatesFlat(
			DDR4Spec.NumRank,
			DDR4Spec.NumBankGroup,
			DDR4Spec.NumBank,
		),
	}
	return state
}

var _ = Describe("Timing Validation", func() {
	var (
		timing    Timing
		cmdCycles map[CommandKind]int
		state     *State
	)

	BeforeEach(func() {
		timing, cmdCycles = buildDDR4TimingAndCycles()
		state = newDDR4State()
	})

	Describe("TestActivateOpensBank", func() {
		It("should open the bank and set CycleLeft to tRCD", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)
			Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))

			cmd := &commandState{
				Kind: int(CmdKindActivate),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       42,
				},
			}

			startCommand(cmdCycles, state, bs, cmd)

			// Bank should now be open with the correct row.
			Expect(BankStateKind(bs.State)).To(Equal(BankStateOpen))
			Expect(bs.OpenRow).To(Equal(uint64(42)))
			// CycleLeft for Activate = tRCD - tAL = 16 - 0 = 16
			Expect(bs.CurrentCmd.CycleLeft).To(Equal(16))
		})
	})

	Describe("TestActivateThenRead", func() {
		It("should allow Read after tRCD cycles", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			activateCmd := &commandState{
				Kind: int(CmdKindActivate),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       100,
				},
			}

			startCommand(cmdCycles, state, bs, activateCmd)
			updateTiming(timing, state, activateCmd)

			// After Activate, the SameBank timing for Activate→Read
			// should be tRCD - tAL = 16.
			readKey := cmdKindToString(CmdKindRead)
			Expect(bs.CyclesToCmdAvailable[readKey]).To(Equal(16))

			// Tick 16 cycles to drain the timing constraint.
			for range 16 {
				tickBank(state, bs)
			}

			// After 16 ticks the Read constraint should be 0.
			Expect(bs.CyclesToCmdAvailable[readKey]).To(Equal(0))

			// The bank command should also have completed (CycleLeft went 16→0).
			Expect(bs.HasCurrentCmd).To(BeFalse())

			// Now a Read command should be ready.
			readCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       100,
				},
			}
			ready := getReadyCommand(&DDR4Spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindRead))
		})
	})

	Describe("TestReadTiming", func() {
		It("should set correct inter-read timing constraints", func() {
			// First open the bank.
			bs00 := findBankState(&state.BankStates, 0, 0, 0)
			activateCmd := &commandState{
				Kind: int(CmdKindActivate),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       1,
				},
			}
			startCommand(cmdCycles, state, bs00, activateCmd)
			updateTiming(timing, state, activateCmd)

			// Tick until Activate completes.
			for range 16 {
				tickBanks(&DDR4Spec, cmdCycles, state)
			}

			// Issue a Read on bank (0,0,0).
			readCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       1,
				},
			}
			startCommand(cmdCycles, state, bs00, readCmd)
			updateTiming(timing, state, readCmd)

			// Same bank: Read→Read constraint should be tCCDL = 6
			readKey := cmdKindToString(CmdKindRead)
			Expect(bs00.CyclesToCmdAvailable[readKey]).To(Equal(6))

			// Other bank in same bank group (e.g., bank index 1 in group 0):
			// should also be tCCDL = 6.
			bs01 := findBankState(&state.BankStates, 0, 0, 1)
			Expect(bs01.CyclesToCmdAvailable[readKey]).To(Equal(6))

			// Bank in a different bank group but same rank (e.g., group 1, bank 0):
			// should be tCCDS = 4.
			bs10 := findBankState(&state.BankStates, 0, 1, 0)
			Expect(bs10.CyclesToCmdAvailable[readKey]).To(Equal(4))
		})
	})

	Describe("TestPrechargeToActivate", func() {
		It("should require tRP cycles before Activate after Precharge", func() {
			// Use a fresh bank with no prior Activate timing residue.
			// We manually set the bank to Open state and clear timing
			// to isolate the Precharge→Activate constraint.
			bs := findBankState(&state.BankStates, 0, 0, 0)
			bs.State = int(BankStateOpen)
			bs.OpenRow = 5

			// Issue Precharge.
			prechargeCmd := &commandState{
				Kind: int(CmdKindPrecharge),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       5,
				},
			}
			startCommand(cmdCycles, state, bs, prechargeCmd)
			updateTiming(timing, state, prechargeCmd)

			// Precharge→Activate constraint should be tRP = 16.
			activateKey := cmdKindToString(CmdKindActivate)
			Expect(bs.CyclesToCmdAvailable[activateKey]).To(Equal(16))

			// Bank should now be closed.
			Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))

			// Tick 16 cycles to drain the constraint.
			for range 16 {
				tickBanks(&DDR4Spec, cmdCycles, state)
			}

			// Now Activate should be ready (constraint == 0).
			Expect(bs.CyclesToCmdAvailable[activateKey]).To(Equal(0))

			// Verify by issuing a Read to the closed bank — getReadyCommand
			// should return Activate (the required predecessor) since the
			// Activate timing constraint has drained.
			readCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       10,
				},
			}
			ready := getReadyCommand(&DDR4Spec, state, bs, readCmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindActivate))
		})
	})

	Describe("TestRowBufferHitVsMiss", func() {
		It("should return Read directly for row hit, Precharge for row miss", func() {
			bs := findBankState(&state.BankStates, 0, 0, 0)

			// Open the bank to row 200.
			activateCmd := &commandState{
				Kind: int(CmdKindActivate),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			startCommand(cmdCycles, state, bs, activateCmd)
			updateTiming(timing, state, activateCmd)

			// Complete the Activate.
			for range 16 {
				tickBanks(&DDR4Spec, cmdCycles, state)
			}

			Expect(BankStateKind(bs.State)).To(Equal(BankStateOpen))
			Expect(bs.OpenRow).To(Equal(uint64(200)))

			// Row buffer HIT: command for same row (200) → should return Read.
			hitCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			hitKind := getRequiredCommandKind(bs, hitCmd)
			Expect(hitKind).To(Equal(CmdKindRead))

			// Row buffer MISS: command for different row (999) → should return Precharge.
			missCmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       999,
				},
			}
			missKind := getRequiredCommandKind(bs, missCmd)
			Expect(missKind).To(Equal(CmdKindPrecharge))

			// Also verify Write hit/miss behaves the same way.
			writeHitCmd := &commandState{
				Kind: int(CmdKindWrite),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			writeHitKind := getRequiredCommandKind(bs, writeHitCmd)
			Expect(writeHitKind).To(Equal(CmdKindWrite))

			writeMissCmd := &commandState{
				Kind: int(CmdKindWrite),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       999,
				},
			}
			writeMissKind := getRequiredCommandKind(bs, writeMissCmd)
			Expect(writeMissKind).To(Equal(CmdKindPrecharge))
		})
	})
})
