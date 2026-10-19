package dram

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTFAWAndRefresh(t *testing.T) {
	var (
		spec      Spec
		st        *state
		cmdCycles map[commandKind]int
		timing    dramTiming
	)

	t.Run("tFAW Constraint", func(t *testing.T) {
		setup1 := func(t *testing.T) {
			t.Helper()

			spec = DDR4Spec()
			spec.TFAW = 28

			timing = generateTiming(&spec)
			cmdCycles = buildCmdCycles(&spec)

			st = &state{
				BankStates: initBankStatesFlat(
					spec.NumRank, spec.NumBankGroup, spec.NumBank),
			}
		}

		t.Run("should block 5th activate within tFAW window", func(t *testing.T) {
			setup1(t)

			// Issue 4 activates on different banks in rank 0.
			// Each at successive ticks so they're all within tFAW.
			for i := range 4 {
				st.TickCount = uint64(i * 2) // spread by 2 ticks
				bs := findBankState(&st.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)
				require.Equal(t, bankStateClosed, bankStateKind(bs.State))

				cmd := &commandState{
					Kind: int(cmdKindActivate),
					Location: location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, st, bs, cmd)
				updateTiming(timing, st, cmd)
			}

			// Now try a 5th activate. The window is tFAW=28 and the
			// oldest activate was at tick 0, current tick is 6 (< 28).
			st.TickCount = 6

			bs := findBankState(&st.BankStates, 0, 0, 1)
			// First close this bank so it can accept an activate
			bs.State = int(bankStateClosed)
			bs.CyclesToCmdAvailable = [numCmdKind]int{} // clear timing

			cmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// getReadyCommand should determine CmdKindActivate is needed
			// (bank is closed) but tFAW should block it.
			ready := getReadyCommand(&spec, st, bs, cmd)
			require.Nil(t, ready)
		})

		t.Run("should allow activate after tFAW window passes", func(t *testing.T) {
			setup1(t)

			// Issue 4 activates at tick 0, 1, 2, 3
			for i := range 4 {
				st.TickCount = uint64(i)
				bs := findBankState(&st.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(cmdKindActivate),
					Location: location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, st, bs, cmd)
				updateTiming(timing, st, cmd)
			}

			// Advance past tFAW window (oldest was at tick 0, tFAW=28)
			st.TickCount = 28

			bs := findBankState(&st.BankStates, 0, 0, 1)
			bs.State = int(bankStateClosed)
			bs.CyclesToCmdAvailable = [numCmdKind]int{}

			cmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// Now the window has passed, activate should be allowed.
			ready := getReadyCommand(&spec, st, bs, cmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindActivate, commandKind(ready.Kind))
		})

		t.Run("should not apply tFAW when TFAW is 0", func(t *testing.T) {
			setup1(t)

			spec.TFAW = 0

			// Issue 4 activates
			for i := range 4 {
				st.TickCount = uint64(i)
				bs := findBankState(&st.BankStates,
					0, i%spec.NumBankGroup, i/spec.NumBankGroup)

				cmd := &commandState{
					Kind: int(cmdKindActivate),
					Location: location{
						Rank:      0,
						BankGroup: uint64(i % spec.NumBankGroup),
						Bank:      uint64(i / spec.NumBankGroup),
						Row:       uint64(100 + i),
					},
				}
				startCommand(cmdCycles, st, bs, cmd)
			}

			st.TickCount = 4
			bs := findBankState(&st.BankStates, 0, 0, 1)
			bs.State = int(bankStateClosed)
			bs.CyclesToCmdAvailable = [numCmdKind]int{}

			cmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      1,
					Row:       999,
				},
			}

			// tFAW=0 means no constraint
			ready := getReadyCommand(&spec, st, bs, cmd)
			require.NotNil(t, ready)
		})
	})

	t.Run("Periodic Refresh", func(t *testing.T) {
		setup2 := func(t *testing.T) {
			t.Helper()

			spec = DDR4Spec()
			spec.TREFI = 20 // small value for testing
			spec.TRFC = 5

			st = &state{
				BankStates: initBankStatesFlat(
					spec.NumRank, spec.NumBankGroup, spec.NumBank),
			}
		}

		t.Run("should trigger refresh after TREFI ticks and stall for TRFC", func(t *testing.T) {
			setup2(t)

			// Simulate the countdown
			require.False(t, st.RefreshInProgress)

			// After TREFI ticks, refresh should trigger
			for i := range spec.TREFI {
				_ = i
				progress := runFakeStallRefresh(&spec, st)
				if i < spec.TREFI-1 {
					require.False(t, st.RefreshInProgress)
				}
				_ = progress
			}

			require.True(t, st.RefreshInProgress)
			require.Equal(t, spec.TRFC, st.RefreshCyclesRemaining)
			require.Equal(t, 0, st.RefreshCycleCounter)
		})

		t.Run("should complete refresh after TRFC cycles", func(t *testing.T) {
			setup2(t)

			// Set up state as if refresh just triggered
			st.RefreshInProgress = true
			st.RefreshCyclesRemaining = spec.TRFC
			st.RefreshCycleCounter = 0

			// Tick TRFC times
			for range spec.TRFC {
				require.True(t, st.RefreshInProgress)
				runFakeStallRefresh(&spec, st)
			}

			// After TRFC ticks, refresh should be complete
			require.False(t, st.RefreshInProgress)
		})

		t.Run("should not trigger refresh when TREFI is 0", func(t *testing.T) {
			setup2(t)

			spec.TREFI = 0

			for range 100 {
				runFakeStallRefresh(&spec, st)
			}

			require.False(t, st.RefreshInProgress)
		})
	})
}
