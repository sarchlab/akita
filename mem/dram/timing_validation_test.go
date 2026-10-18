package dram

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// buildDDR4TimingAndCycles generates the Timing and cmdCycles for the DDR4
// spec, as the component does when it is built, without constructing a full
// Component. The derived values for DDR4 are:
//
//	burstCycle = burstLength / 2 = 8 / 2 = 4
//	tRL        = tAL + tCL        = 0 + 16 = 16
//	tWL        = tAL + tCWL       = 0 + 12 = 12
//	readDelay  = tRL + burstCycle = 16 + 4 = 20
//	writeDelay = tRL + burstCycle = 16 + 4 = 20
//	tRC        = tRAS + tRP       = 39 + 16 = 55
func buildDDR4TimingAndCycles() (dramTiming, map[commandKind]int) {
	spec := DDR4Spec()

	return generateTiming(&spec), buildCmdCycles(&spec)
}

// newDDR4State creates a fresh State with DDR4 bank layout (1 rank, 4 bank groups, 4 banks).
func newDDR4State() *state {
	spec := DDR4Spec()
	st := &state{
		BankStates: initBankStatesFlat(
			spec.NumRank,
			spec.NumBankGroup,
			spec.NumBank,
		),
	}
	return st
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestTimingValidation(t *testing.T) {
	var (
		timing    dramTiming
		cmdCycles map[commandKind]int
		st        *state
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		timing, cmdCycles = buildDDR4TimingAndCycles()
		st = newDDR4State()
	}

	t.Run("TestActivateOpensBank", func(t *testing.T) {
		t.Run("should open the bank and require tRCD before a Read", func(t *testing.T) {
			setup1(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)
			require.Equal(t, bankStateClosed, bankStateKind(bs.State))

			cmd := &commandState{
				Kind: int(cmdKindActivate),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       42,
				},
			}

			startCommand(cmdCycles, st, bs, cmd)
			updateTiming(timing, st, cmd)

			// Bank should now be open with the correct row.
			require.Equal(t, bankStateOpen, bankStateKind(bs.State))
			require.Equal(t, uint64(42), bs.OpenRow)
			// Activate→Read gap = tRCD - tAL = 16 - 0 = 16
			require.Equal(t, 16, bs.CyclesToCmdAvailable[cmdKindRead])
		})
	})

	t.Run("TestActivateThenRead", func(t *testing.T) {
		t.Run("should allow Read after tRCD cycles", func(t *testing.T) {
			setup1(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			activateCmd := &commandState{
				Kind: int(cmdKindActivate),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       100,
				},
			}

			startCommand(cmdCycles, st, bs, activateCmd)
			updateTiming(timing, st, activateCmd)

			// After Activate, the SameBank timing for Activate→Read
			// should be tRCD - tAL = 16.
			readKey := cmdKindRead
			require.Equal(t, 16, bs.CyclesToCmdAvailable[readKey])

			// Tick 16 cycles to drain the timing constraint.
			for range 16 {
				tickBank(bs)
			}

			// After 16 ticks the Read constraint should be 0.
			require.Equal(t, 0, bs.CyclesToCmdAvailable[readKey])

			// Now a Read command should be ready.
			readCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       100,
				},
			}
			spec := DDR4Spec()
			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindRead, commandKind(ready.Kind))
		})
	})

	t.Run("TestReadTiming", func(t *testing.T) {
		t.Run("should set correct inter-read timing constraints", func(t *testing.T) {
			setup1(t)

			// First open the bank.
			bs00 := findBankState(&st.BankStates, 0, 0, 0)
			activateCmd := &commandState{
				Kind: int(cmdKindActivate),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       1,
				},
			}
			startCommand(cmdCycles, st, bs00, activateCmd)
			updateTiming(timing, st, activateCmd)

			// Tick until Activate completes.
			for range 16 {
				tickBanks(st)
			}

			// Issue a Read on bank (0,0,0).
			readCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       1,
				},
			}
			startCommand(cmdCycles, st, bs00, readCmd)
			updateTiming(timing, st, readCmd)

			// Same bank: Read→Read constraint should be tCCDL = 6
			readKey := cmdKindRead
			require.Equal(t, 6, bs00.CyclesToCmdAvailable[readKey])

			// Other bank in same bank group (e.g., bank index 1 in group 0):
			// should also be tCCDL = 6.
			bs01 := findBankState(&st.BankStates, 0, 0, 1)
			require.Equal(t, 6, bs01.CyclesToCmdAvailable[readKey])

			// Bank in a different bank group but same rank (e.g., group 1, bank 0):
			// should be tCCDS = 4.
			bs10 := findBankState(&st.BankStates, 0, 1, 0)
			require.Equal(t, 4, bs10.CyclesToCmdAvailable[readKey])
		})
	})

	t.Run("TestPrechargeToActivate", func(t *testing.T) {
		t.Run("should require tRP cycles before Activate after Precharge", func(t *testing.T) {
			setup1(t)

			// Use a fresh bank with no prior Activate timing residue.
			// We manually set the bank to Open state and clear timing
			// to isolate the Precharge→Activate constraint.
			bs := findBankState(&st.BankStates, 0, 0, 0)
			bs.State = int(bankStateOpen)
			bs.OpenRow = 5

			// Issue Precharge.
			prechargeCmd := &commandState{
				Kind: int(cmdKindPrecharge),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       5,
				},
			}
			startCommand(cmdCycles, st, bs, prechargeCmd)
			updateTiming(timing, st, prechargeCmd)

			// Precharge→Activate constraint should be tRP = 16.
			activateKey := cmdKindActivate
			require.Equal(t, 16, bs.CyclesToCmdAvailable[activateKey])

			// Bank should now be closed.
			require.Equal(t, bankStateClosed, bankStateKind(bs.State))

			// Tick 16 cycles to drain the constraint.
			for range 16 {
				tickBanks(st)
			}

			// Now Activate should be ready (constraint == 0).
			require.Equal(t, 0, bs.CyclesToCmdAvailable[activateKey])

			// Verify by issuing a Read to the closed bank — getReadyCommand
			// should return Activate (the required predecessor) since the
			// Activate timing constraint has drained.
			readCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       10,
				},
			}
			spec := DDR4Spec()
			ready := getReadyCommand(&spec, st, bs, readCmd)
			require.NotNil(t, ready)
			require.Equal(t, cmdKindActivate, commandKind(ready.Kind))
		})
	})

	t.Run("TestRowBufferHitVsMiss", func(t *testing.T) {
		t.Run("should return Read directly for row hit, Precharge for row miss", func(t *testing.T) {
			setup1(t)

			bs := findBankState(&st.BankStates, 0, 0, 0)

			// Open the bank to row 200.
			activateCmd := &commandState{
				Kind: int(cmdKindActivate),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			startCommand(cmdCycles, st, bs, activateCmd)
			updateTiming(timing, st, activateCmd)

			// Complete the Activate.
			for range 16 {
				tickBanks(st)
			}

			require.Equal(t, bankStateOpen, bankStateKind(bs.State))
			require.Equal(t, uint64(200), bs.OpenRow)

			// Row buffer HIT: command for same row (200) → should return Read.
			hitCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			hitKind := getRequiredCommandKind(bs, hitCmd)
			require.Equal(t, cmdKindRead, hitKind)

			// Row buffer MISS: command for different row (999) → should return Precharge.
			missCmd := &commandState{
				Kind: int(cmdKindRead),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       999,
				},
			}
			missKind := getRequiredCommandKind(bs, missCmd)
			require.Equal(t, cmdKindPrecharge, missKind)

			// Also verify Write hit/miss behaves the same way.
			writeHitCmd := &commandState{
				Kind: int(cmdKindWrite),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       200,
				},
			}
			writeHitKind := getRequiredCommandKind(bs, writeHitCmd)
			require.Equal(t, cmdKindWrite, writeHitKind)

			writeMissCmd := &commandState{
				Kind: int(cmdKindWrite),
				Location: location{
					Rank:      0,
					BankGroup: 0,
					Bank:      0,
					Row:       999,
				},
			}
			writeMissKind := getRequiredCommandKind(bs, writeMissCmd)
			require.Equal(t, cmdKindPrecharge, writeMissKind)
		})
	})
}
