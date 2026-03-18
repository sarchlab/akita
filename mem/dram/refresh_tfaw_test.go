package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tFAW and Refresh", func() {
	var (
		spec      Spec
		state     *State
		cmdCycles map[CommandKind]int
		timing    Timing
	)

	Describe("tFAW Constraint", func() {
		BeforeEach(func() {
			spec = DDR4Spec
			spec.TFAW = 28

			b := MakeBuilder().WithSpec(spec)
			b.burstCycle = b.burstLength / 2
			b.tRL = b.tAL + b.tCL
			b.tWL = b.tAL + b.tCWL
			b.readDelay = b.tRL + b.burstCycle
			b.writeDelay = b.tRL + b.burstCycle
			b.tRC = b.tRAS + b.tRP

			timing = b.generateTiming()
			cmdCycles = b.buildCmdCycles()

			state = &State{
				BankStates: initBankStatesFlat(
					spec.NumRank, spec.NumBankGroup, spec.NumBank),
			}
		})

		It("should block 5th activate within tFAW window", func() {
			// Issue 4 activates on different banks in rank 0.
			// Each at successive ticks so they're all within tFAW.
			for i := range 4 {
				state.TickCount = uint64(i * 2) // spread by 2 ticks
				bs := findBankState(&state.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)
				Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))

				cmd := &commandState{
					Kind: int(CmdKindActivate),
					Location: Location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, state, bs, cmd)
				updateTiming(timing, state, cmd)
			}

			// Now try a 5th activate. The window is tFAW=28 and the
			// oldest activate was at tick 0, current tick is 6 (< 28).
			state.TickCount = 6

			bs := findBankState(&state.BankStates, 0, 0, 1)
			// First close this bank so it can accept an activate
			bs.State = int(BankStateClosed)
			bs.HasCurrentCmd = false
			bs.CyclesToCmdAvailable = make(map[string]int) // clear timing

			cmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// getReadyCommand should determine CmdKindActivate is needed
			// (bank is closed) but tFAW should block it.
			ready := getReadyCommand(&spec, state, bs, cmd)
			Expect(ready).To(BeNil())
		})

		It("should allow activate after tFAW window passes", func() {
			// Issue 4 activates at tick 0, 1, 2, 3
			for i := range 4 {
				state.TickCount = uint64(i)
				bs := findBankState(&state.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(CmdKindActivate),
					Location: Location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, state, bs, cmd)
				updateTiming(timing, state, cmd)
			}

			// Advance past tFAW window (oldest was at tick 0, tFAW=28)
			state.TickCount = 28

			bs := findBankState(&state.BankStates, 0, 0, 1)
			bs.State = int(BankStateClosed)
			bs.HasCurrentCmd = false
			bs.CyclesToCmdAvailable = make(map[string]int)

			cmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// Now the window has passed, activate should be allowed.
			ready := getReadyCommand(&spec, state, bs, cmd)
			Expect(ready).NotTo(BeNil())
			Expect(CommandKind(ready.Kind)).To(Equal(CmdKindActivate))
		})

		It("should not apply tFAW when TFAW is 0", func() {
			spec.TFAW = 0

			// Issue 4 activates
			for i := range 4 {
				state.TickCount = uint64(i)
				bs := findBankState(&state.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(CmdKindActivate),
					Location: Location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, state, bs, cmd)
			}

			state.TickCount = 4
			bs := findBankState(&state.BankStates, 0, 0, 1)
			bs.State = int(BankStateClosed)
			bs.HasCurrentCmd = false
			bs.CyclesToCmdAvailable = make(map[string]int)

			cmd := &commandState{
				Kind: int(CmdKindRead),
				Location: Location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// tFAW=0 means no constraint
			ready := getReadyCommand(&spec, state, bs, cmd)
			Expect(ready).NotTo(BeNil())
		})
	})

	Describe("Periodic Refresh", func() {
		BeforeEach(func() {
			spec = DDR4Spec
			spec.TREFI = 20 // small value for testing
			spec.TRFC = 5

			state = &State{
				BankStates: initBankStatesFlat(
					spec.NumRank, spec.NumBankGroup, spec.NumBank),
			}
		})

		It("should trigger refresh after TREFI ticks and stall for TRFC", func() {
			mw := &bankTickMW{
				CmdCycles: map[CommandKind]int{},
			}

			// Simulate the countdown
			Expect(state.RefreshInProgress).To(BeFalse())

			// After TREFI ticks, refresh should trigger
			for i := range spec.TREFI {
				_ = i
				progress := mw.handleRefresh(&spec, state)
				if i < spec.TREFI-1 {
					Expect(state.RefreshInProgress).To(BeFalse())
				}
				_ = progress
			}

			Expect(state.RefreshInProgress).To(BeTrue())
			Expect(state.RefreshCyclesRemaining).To(Equal(spec.TRFC))
			Expect(state.RefreshCycleCounter).To(Equal(0))
		})

		It("should complete refresh after TRFC cycles", func() {
			// Set up state as if refresh just triggered
			state.RefreshInProgress = true
			state.RefreshCyclesRemaining = spec.TRFC
			state.RefreshCycleCounter = 0

			mw := &bankTickMW{
				CmdCycles: map[CommandKind]int{},
			}

			// Tick TRFC times
			for range spec.TRFC {
				Expect(state.RefreshInProgress).To(BeTrue())
				mw.handleRefresh(&spec, state)
			}

			// After TRFC ticks, refresh should be complete
			Expect(state.RefreshInProgress).To(BeFalse())
		})

		It("should not trigger refresh when TREFI is 0", func() {
			spec.TREFI = 0

			mw := &bankTickMW{
				CmdCycles: map[CommandKind]int{},
			}

			for range 100 {
				mw.handleRefresh(&spec, state)
			}

			Expect(state.RefreshInProgress).To(BeFalse())
		})
	})
})
