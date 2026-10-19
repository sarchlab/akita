package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestAddressOperations(t *testing.T) {
	t.Run("should map address", func(t *testing.T) {
		spec := DefaultSpec()

		loc := newAddrMapping(&spec).mapAddress(0)
		require.Equal(t, uint64(0), loc.Channel)
		require.Equal(t, uint64(0), loc.Rank)
	})
}

func TestTransactionSplitting(t *testing.T) {
	var ids *testsim.Simulation
	setup1 := func(t *testing.T) {
		t.Helper()
		ids = akitasim.MakeBuilder().Build()
	}
	t.Run("should split a transaction into sub-transactions", func(t *testing.T) {
		setup1(t)

		spec := &Spec{BusWidth: 64, BurstLength: 8} // 64-byte access unit
		trans := &transactionState{
			HasRead: true,
			ReadMsg: messaging.Msg{Payload: memprotocol.ReadReq{}},
		}
		payload1 := trans.ReadMsg.Payload.(memprotocol.ReadReq)
		payload1.Address = 0x100
		trans.ReadMsg.Payload = payload1
		payload2 := trans.ReadMsg.Payload.(memprotocol.ReadReq)
		payload2.AccessByteSize = 128
		trans.ReadMsg.Payload = payload2

		splitTransaction(ids.NewID, spec, trans)
		// 128 bytes at 64-byte units = 2 sub-transactions
		require.Len(t, trans.SubTransactions, 2)
		require.Equal(t, uint64(0x100), trans.SubTransactions[0].Address)
		require.Equal(t, uint64(0x140), trans.SubTransactions[1].Address)
	})

	t.Run("should align to unit boundaries", func(t *testing.T) {
		setup1(t)

		spec := &Spec{BusWidth: 64, BurstLength: 8} // 64-byte access unit
		trans := &transactionState{
			HasRead: true,
			ReadMsg: messaging.Msg{Payload: memprotocol.ReadReq{}},
		}
		payload3 := trans.ReadMsg.Payload.(memprotocol.ReadReq)
		payload3.Address = 0x110
		trans.ReadMsg.Payload = payload3
		payload4 := // Not aligned
			trans.ReadMsg.Payload.(memprotocol.ReadReq)
		payload4.AccessByteSize = 4
		trans.ReadMsg.Payload = payload4

		splitTransaction(ids.NewID, spec, trans)
		require.Len(t, trans.SubTransactions, 1)
		require.Equal(t, uint64(0x100), trans.SubTransactions[0].Address)
	})
}

func TestBankOperations(t *testing.T) {
	t.Run("should get required command kind for closed bank", func(t *testing.T) {
		bs := &bankState{
			State:                int(bankStateClosed),
			CyclesToCmdAvailable: [numCmdKind]int{},
		}
		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		require.Equal(t, cmdKindActivate, requiredKind)
	})

	t.Run("should get required command kind for open bank - same row", func(t *testing.T) {
		bs := &bankState{
			State:                int(bankStateOpen),
			OpenRow:              10,
			CyclesToCmdAvailable: [numCmdKind]int{},
		}
		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		require.Equal(t, cmdKindReadPrecharge, requiredKind)
	})

	t.Run("should get precharge for open bank - different row", func(t *testing.T) {
		bs := &bankState{
			State:                int(bankStateOpen),
			OpenRow:              5,
			CyclesToCmdAvailable: [numCmdKind]int{},
		}
		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		require.Equal(t, cmdKindPrecharge, requiredKind)
	})

	t.Run("should tick banks and count down timing gaps", func(t *testing.T) {
		st := &state{
			BankStates: bankStatesFlat{
				NumRanks:      1,
				NumBankGroups: 1,
				NumBanks:      1,
				Entries: []bankEntry{
					{
						Rank: 0, BankGroup: 0, BankIndex: 0,
						Data: bankState{
							State: int(bankStateClosed),
							CyclesToCmdAvailable: [numCmdKind]int{
								cmdKindRead: 3,
							},
						},
					},
				},
			},
		}

		progress := tickBanks(st)
		require.True(t, progress)
		bs := &st.BankStates.Entries[0].Data
		require.Equal(t, 2, bs.CyclesToCmdAvailable[cmdKindRead])
	})

	t.Run("should complete pending reads/writes and mark subtrans done", func(t *testing.T) {
		st := &state{
			TickCount: 100,
			Transactions: []transactionState{
				{
					ID:      7,
					HasRead: true,
					SubTransactions: []subTransState{
						{ID: 1, Completed: false},
					},
				},
			},
			PendingCompletions: []pendingCompletion{
				{
					CompletionTick: 100,
					Ref:            subTransRef{TxID: 7, SubIndex: 0},
				},
			},
		}

		completed := processPendingCompletions(st)

		require.NotEmpty(t, completed)
		require.Empty(t, st.PendingCompletions)
		require.True(t, st.Transactions[0].SubTransactions[0].Completed)
	})
}

func TestQueueOperations(t *testing.T) {
	t.Run("should check if sub-trans queue can push", func(t *testing.T) {
		st := &state{
			SubTransQueue: subTransQueueState{
				Entries: make([]subTransRef, 5),
			},
		}
		require.True(t, canPushSubTrans(st, 3, 10))
		require.False(t, canPushSubTrans(st, 6, 10))
	})

	t.Run("should push sub-transactions", func(t *testing.T) {
		st := &state{
			Transactions: []transactionState{
				{
					SubTransactions: []subTransState{
						{ID: 2},
						{ID: 1},
					},
				},
			},
			SubTransQueue: subTransQueueState{
				Entries: []subTransRef{},
			},
		}

		pushSubTrans(st, 0)
		require.Len(t, st.SubTransQueue.Entries, 2)
		require.Equal(t, subTransRef{TxID: 0, SubIndex: 0}, st.SubTransQueue.Entries[0])
	})
}

func TestDRAMIntegration(t *testing.T) {
	var (
		engine  timing.Engine
		sim     *testsim.Simulation
		memCtrl *Comp
	)

	setup2 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		memCtrl = buildDRAM(sim, DefaultSpec(), "MemCtrl", 1024)
	}

	t.Run("should read and write via direct connection", func(t *testing.T) {
		setup2(t)

		srcPort := newDriverPort(sim, "Src.Top", 1024)
		conn := direct.NewConnection("Conn", sim, timing.GHz)
		topPort := memCtrl.Ports.Top
		conn.BindPort(topPort)
		conn.BindPort(srcPort)
		require.NoError(t, sim.Initialize())

		writeData := []byte{1, 2, 3, 4}
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x40,
			Data:    writeData},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: len(writeData) + 12,
		}

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x40,
			AccessByteSize: 4},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
		}

		srcPort.Send(write)
		srcPort.Send(read)

		require.NoError(t, engine.Run())

		// Collect responses
		var writeDone messaging.Msg
		var dataReady messaging.Msg
		var gotWriteDone, gotDataReady bool

		for {
			msg, ok := srcPort.RetrieveIncoming()
			if !ok {
				break
			}
			switch msg.Payload.(type) {
			case memprotocol.WriteDoneRsp:
				m := msg

				writeDone = m
				gotWriteDone = true
			case memprotocol.DataReadyRsp:
				m := msg

				dataReady = m
				gotDataReady = true
			}
		}

		require.True(t, gotWriteDone)
		require.Equal(t, write.ID, writeDone.RspTo)
		require.True(t, gotDataReady)
		require.Equal(t, read.ID, dataReady.RspTo)
		require.Equal(t, []byte{1, 2, 3, 4}, dataReady.Payload.(memprotocol.DataReadyRsp).Data)
	})
}

// ========================================================================
// New feature tests below
// ========================================================================

func TestPredefinedSpecs(t *testing.T) {
	t.Run("should build with DDR4 spec", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		ctrl := buildDRAM(sim, DDR4Spec(), "DDR4Ctrl", 16)
		require.NotNil(t, ctrl)
		spec := ctrl.Spec
		require.Equal(t, 8, spec.BurstLength)
		require.Equal(t, 4, spec.NumBankGroup)
		require.Equal(t, 4, spec.NumBank)
		require.Equal(t, 1, spec.NumRank)
		require.Equal(t, 64, spec.BusWidth)
		require.Equal(t, int(protoDDR4), spec.Protocol)
	})

	t.Run("should build with DDR5 spec", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		ctrl := buildDRAM(sim, DDR5Spec(), "DDR5Ctrl", 16)
		require.NotNil(t, ctrl)
		spec := ctrl.Spec
		require.Equal(t, 16, spec.BurstLength)
		require.Equal(t, 8, spec.NumBankGroup)
		require.Equal(t, 4, spec.NumBank)
		require.Equal(t, 32, spec.BusWidth)
		require.Equal(t, int(protoDDR5), spec.Protocol)
	})

	t.Run("should build with HBM2 spec", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		ctrl := buildDRAM(sim, HBM2Spec(), "HBM2Ctrl", 16)
		require.NotNil(t, ctrl)
		spec := ctrl.Spec
		require.Equal(t, 4, spec.BurstLength)
		require.Equal(t, 4, spec.NumBankGroup)
		require.Equal(t, 4, spec.NumBank)
		require.Equal(t, 128, spec.BusWidth)
		require.Equal(t, 128, spec.DeviceWidth)
		require.Equal(t, int(protoHBM2), spec.Protocol)
	})

	t.Run("should build with HBM3 spec", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		ctrl := buildDRAM(sim, HBM3Spec(), "HBM3Ctrl", 16)
		require.NotNil(t, ctrl)
		spec := ctrl.Spec
		require.Equal(t, 8, spec.BurstLength)
		require.Equal(t, 4, spec.NumBankGroup)
		require.Equal(t, 64, spec.BusWidth)
		require.Equal(t, 64, spec.DeviceWidth)
		require.Equal(t, int(protoHBM3), spec.Protocol)
	})

	t.Run("should build with GDDR6 spec", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		ctrl := buildDRAM(sim, GDDR6Spec(), "GDDR6Ctrl", 16)
		require.NotNil(t, ctrl)
		spec := ctrl.Spec
		require.Equal(t, 16, spec.BurstLength)
		require.Equal(t, 4, spec.NumBankGroup)
		require.Equal(t, 4, spec.NumBank)
		require.Equal(t, 32, spec.BusWidth)
		require.Equal(t, 32, spec.DeviceWidth)
		require.Equal(t, int(protoGDDR6), spec.Protocol)
	})

	t.Run("should preserve DDR4 timing parameters", func(t *testing.T) {
		require.Equal(t, 16, DDR4Spec().TCL)
		require.Equal(t, 16, DDR4Spec().TRCD)
		require.Equal(t, 16, DDR4Spec().TRP)
		require.Equal(t, 39, DDR4Spec().TRAS)
	})

	t.Run("should preserve DDR5 timing parameters", func(t *testing.T) {
		require.Equal(t, 40, DDR5Spec().TCL)
		require.Equal(t, 40, DDR5Spec().TRCD)
		require.Equal(t, 40, DDR5Spec().TRP)
		require.Equal(t, 76, DDR5Spec().TRAS)
	})

	t.Run("should preserve HBM3 timing parameters", func(t *testing.T) {
		require.Equal(t, 36, HBM3Spec().TCL)
		require.Equal(t, 36, HBM3Spec().TRCD)
		require.Equal(t, 36, HBM3Spec().TRP)
		require.Equal(t, 72, HBM3Spec().TRAS)
		require.Equal(t, 36, HBM3Spec().TRCDRD)
		require.Equal(t, 24, HBM3Spec().TRCDWR)
	})
}

func TestProtocolEnums(t *testing.T) {
	t.Run("should have new protocol values after HMC", func(t *testing.T) {
		require.Greater(t, int(protoDDR5), int(protoHMC))
		require.Greater(t, int(protoHBM3), int(protoDDR5))
		require.Greater(t, int(protoLPDDR5), int(protoHBM3))
		require.Greater(t, int(protoHBM3E), int(protoLPDDR5))
	})

	t.Run("should recognize HBM3 as HBM", func(t *testing.T) {
		require.True(t, protoHBM3.isHBM())
	})

	t.Run("should recognize HBM3E as HBM", func(t *testing.T) {
		require.True(t, protoHBM3E.isHBM())
	})

	t.Run("should recognize HBM2 as HBM", func(t *testing.T) {
		require.True(t, protoHBM2.isHBM())
	})

	t.Run("should recognize HBM as HBM", func(t *testing.T) {
		require.True(t, protoHBM.isHBM())
	})

	t.Run("should not recognize DDR4 as HBM", func(t *testing.T) {
		require.False(t, protoDDR4.isHBM())
	})

	t.Run("should not recognize DDR5 as HBM", func(t *testing.T) {
		require.False(t, protoDDR5.isHBM())
	})

	t.Run("should recognize GDDR6 as GDDR", func(t *testing.T) {
		require.True(t, protoGDDR6.isGDDR())
	})

	t.Run("should recognize GDDR5 as GDDR", func(t *testing.T) {
		require.True(t, protoGDDR5.isGDDR())
	})

	t.Run("should recognize GDDR5X as GDDR", func(t *testing.T) {
		require.True(t, protoGDDR5X.isGDDR())
	})

	t.Run("should not recognize DDR4 as GDDR", func(t *testing.T) {
		require.False(t, protoDDR4.isGDDR())
	})

	t.Run("should not recognize HBM3 as GDDR", func(t *testing.T) {
		require.False(t, protoHBM3.isGDDR())
	})

	t.Run("should have all original protocols in correct order", func(t *testing.T) {
		require.Equal(t, 0, int(protoDDR3))
		require.Equal(t, 1, int(protoDDR4))
		require.Equal(t, 2, int(protoGDDR5))
		require.Equal(t, 3, int(protoGDDR5X))
		require.Equal(t, 4, int(protoGDDR6))
		require.Equal(t, 5, int(protoLPDDR))
		require.Equal(t, 6, int(protoLPDDR3))
		require.Equal(t, 7, int(protoLPDDR4))
		require.Equal(t, 8, int(protoHBM))
		require.Equal(t, 9, int(protoHBM2))
		require.Equal(t, 10, int(protoHMC))
	})
}

func TestOpenPagePolicy(t *testing.T) {
	var (
		ids  *testsim.Simulation
		spec *Spec
		st   *state
	)

	setup3 := func(t *testing.T) {
		t.Helper()

		ids = akitasim.MakeBuilder().Build()
		defaultSpec := DefaultSpec()
		spec = &defaultSpec
		st = &state{
			Transactions: []transactionState{
				{
					ID:      0,
					HasRead: true,
					ReadMsg: messaging.Msg{Payload: memprotocol.ReadReq{}},
					SubTransactions: []subTransState{
						{
							ID:        10,
							Address:   0x0,
							Completed: false,
						},
					},
				},
				{
					ID:       1,
					HasWrite: true,
					WriteMsg: messaging.Msg{Payload: memprotocol.WriteReq{}},
					SubTransactions: []subTransState{
						{
							ID:        20,
							Address:   0x0,
							Completed: false,
						},
					},
				},
			},
			SubTransQueue: subTransQueueState{Entries: []subTransRef{}},
			CommandQueues: commandQueueState{
				NumQueues: 1,
				Entries:   []queueEntry{},
			},
			BankStates: initBankStatesFlat(
				spec.NumRank, spec.NumBankGroup, spec.NumBank),
		}
	}

	t.Run("should create open page command with Read kind", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createOpenPageCommand(ids.NewID, spec, st, ref)

		require.NotNil(t, cmd)
		require.Equal(t, int(cmdKindRead), cmd.Kind)
	})

	t.Run("should create open page command with Write kind", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createOpenPageCommand(ids.NewID, spec, st, ref)

		require.NotNil(t, cmd)
		require.Equal(t, int(cmdKindWrite), cmd.Kind)
	})

	t.Run("should create close page command with ReadPrecharge kind", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createClosePageCommand(ids.NewID, spec, st, ref)

		require.NotNil(t, cmd)
		require.Equal(t, int(cmdKindReadPrecharge), cmd.Kind)
	})

	t.Run("should create close page command with WritePrecharge kind", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createClosePageCommand(ids.NewID, spec, st, ref)

		require.NotNil(t, cmd)
		require.Equal(t, int(cmdKindWritePrecharge), cmd.Kind)
	})

	t.Run("should keep bank open after Read command in open-page mode", func(t *testing.T) {
		setup3(t)

		// Set up a bank that is Open at row 5
		bs := &st.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindRead),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindRead: 10,
		}

		startCommand(cmdCycles, st, bs, cmd)

		// Bank should remain open
		require.Equal(t, bankStateOpen, bankStateKind(bs.State))
		require.Equal(t, uint64(5), bs.OpenRow)
	})

	t.Run("should keep bank open after Write command in open-page mode", func(t *testing.T) {
		setup3(t)

		bs := &st.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 7

		cmd := &commandState{
			Kind:     int(cmdKindWrite),
			Location: location{Row: 7, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindWrite: 10,
		}

		startCommand(cmdCycles, st, bs, cmd)

		// Bank should remain open
		require.Equal(t, bankStateOpen, bankStateKind(bs.State))
		require.Equal(t, uint64(7), bs.OpenRow)
	})

	t.Run("should close bank after ReadPrecharge command", func(t *testing.T) {
		setup3(t)

		bs := &st.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindReadPrecharge: 10,
		}

		startCommand(cmdCycles, st, bs, cmd)

		// Bank should be closed
		require.Equal(t, bankStateClosed, bankStateKind(bs.State))
	})

	t.Run("should close bank after WritePrecharge command", func(t *testing.T) {
		setup3(t)

		bs := &st.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindWritePrecharge: 10,
		}

		startCommand(cmdCycles, st, bs, cmd)

		// Bank should be closed
		require.Equal(t, bankStateClosed, bankStateKind(bs.State))
	})

	t.Run("should select command creation based on page policy in tickSubTransQueue", func(t *testing.T) {
		setup3(t)

		// Test with open-page policy
		spec.PagePolicy = PagePolicyOpen
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		require.True(t, progress)

		// The command in the queue should be CmdKindRead (not ReadPrecharge)
		require.Len(t, st.CommandQueues.Entries, 1)
		require.Equal(t, int(cmdKindRead), st.CommandQueues.Entries[0].Command.Kind)
	})

	t.Run("should use close-page commands when PagePolicyClose in tickSubTransQueue", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyClose
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		require.True(t, progress)

		// The command in the queue should be CmdKindReadPrecharge
		require.Len(t, st.CommandQueues.Entries, 1)
		require.Equal(t, int(cmdKindReadPrecharge), st.CommandQueues.Entries[0].Command.Kind)
	})

	t.Run("should use Write for write transaction with open-page in tickSubTransQueue", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyOpen
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		require.True(t, progress)

		require.Len(t, st.CommandQueues.Entries, 1)
		require.Equal(t, int(cmdKindWrite), st.CommandQueues.Entries[0].Command.Kind)
	})

	t.Run("should use WritePrecharge for write transaction with close-page in tickSubTransQueue", func(t *testing.T) {
		setup3(t)

		spec.PagePolicy = PagePolicyClose
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		require.True(t, progress)

		require.Len(t, st.CommandQueues.Entries, 1)
		require.Equal(t, int(cmdKindWritePrecharge), st.CommandQueues.Entries[0].Command.Kind)
	})
}

func TestFRFCFSScheduling(t *testing.T) {
	var (
		spec *Spec
		st   *state
	)

	setup4 := func(t *testing.T) {
		t.Helper()

		spec = &Spec{
			NumRank:              1,
			NumBankGroup:         1,
			NumBank:              2,
			CommandQueueCapacity: 16,
		}
		st = &state{
			CommandQueues: commandQueueState{
				NumQueues: 1,
				Entries:   []queueEntry{},
			},
			BankStates: initBankStatesFlat(1, 1, 2),
		}
	}

	t.Run("should prioritize row-buffer hits over misses", func(t *testing.T) {
		setup4(t)

		// Open bank 0 at row 5
		bs0 := findBankState(&st.BankStates, 0, 0, 0)
		bs0.State = int(bankStateOpen)
		bs0.OpenRow = 5
		bs0.CyclesToCmdAvailable = [numCmdKind]int{}

		// Command A: targets row 10 on bank 0 (miss — needs precharge)
		cmdA := commandState{
			ID:   100,
			Kind: int(cmdKindReadPrecharge),
			Location: location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 10,
			},
		}
		// Command B: targets row 5 on bank 0 (hit — matching row)
		cmdB := commandState{
			ID:   101,
			Kind: int(cmdKindReadPrecharge),
			Location: location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 5,
			},
		}

		// Add A first (older), then B (newer)
		st.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmdA},
			{QueueIndex: 0, Command: cmdB},
		}

		result := getCommandToIssue(spec, st)
		require.NotNil(t, result)
		// Should pick the hit (row 5) even though it was added second
		require.Equal(t, uint64(5), result.Location.Row)
	})

	t.Run("should use FCFS when no row-buffer hits", func(t *testing.T) {
		setup4(t)

		// Both banks closed — no row-buffer hits possible
		bs0 := findBankState(&st.BankStates, 0, 0, 0)
		bs0.CyclesToCmdAvailable = [numCmdKind]int{}
		bs1 := findBankState(&st.BankStates, 0, 0, 1)
		bs1.CyclesToCmdAvailable = [numCmdKind]int{}

		cmdA := commandState{
			ID:   102,
			Kind: int(cmdKindReadPrecharge),
			Location: location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 10,
			},
		}
		cmdB := commandState{
			ID:   103,
			Kind: int(cmdKindReadPrecharge),
			Location: location{
				Rank: 0, BankGroup: 0, Bank: 1, Row: 20,
			},
		}

		st.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmdA},
			{QueueIndex: 0, Command: cmdB},
		}

		result := getCommandToIssue(spec, st)
		require.NotNil(t, result)
		// For closed banks, getRequiredCommandKind returns Activate.
		// The ready command should be an Activate for the older command.
		require.Equal(t, int(cmdKindActivate), result.Kind)
		require.Equal(t, uint64(10), result.Location.Row)
	})

	t.Run("should handle empty queue", func(t *testing.T) {
		setup4(t)

		st.CommandQueues.Entries = []queueEntry{}

		result := getCommandToIssue(spec, st)
		require.Nil(t, result)
	})

	t.Run("should return nil when all commands have timing constraints", func(t *testing.T) {
		setup4(t)

		bs0 := findBankState(&st.BankStates, 0, 0, 0)
		bs0.CyclesToCmdAvailable = [numCmdKind]int{
			cmdKindActivate:      5,
			cmdKindReadPrecharge: 5,
			cmdKindRead:          5,
			cmdKindPrecharge:     5,
		}

		cmd := commandState{
			ID:   104,
			Kind: int(cmdKindReadPrecharge),
			Location: location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 10,
			},
		}
		st.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmd},
		}

		result := getCommandToIssue(spec, st)
		require.Nil(t, result)
	})
}

func TestReadWriteQueueSeparation(t *testing.T) {
	var (
		ids  *testsim.Simulation
		spec *Spec
		st   *state
	)

	setup5 := func(t *testing.T) {
		t.Helper()

		ids = akitasim.MakeBuilder().Build()
		spec = &Spec{
			NumRank:              1,
			NumBankGroup:         1,
			NumBank:              4,
			CommandQueueCapacity: 16,
			ReadQueueSize:        4,
			WriteQueueSize:       4,
			WriteHighWatermark:   4,
			WriteLowWatermark:    2,
		}
		st = &state{
			CommandQueues: commandQueueState{
				NumQueues: 1,
				Entries:   []queueEntry{},
			},
			BankStates: initBankStatesFlat(1, 1, 4),
		}
	}

	t.Run("should count write commands", func(t *testing.T) {
		setup5(t)

		st.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWritePrecharge)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
		}

		count := countWriteCommands(st)
		require.Equal(t, 3, count)
	})

	t.Run("should count zero writes in empty queue", func(t *testing.T) {
		setup5(t)

		count := countWriteCommands(st)
		require.Equal(t, 0, count)
	})

	t.Run("should enter drain mode at high watermark", func(t *testing.T) {
		setup5(t)

		// Set up 4 open banks, each with a write command that's a row hit
		for i := range 4 {
			bs := findBankState(&st.BankStates, 0, 0, i)
			bs.State = int(bankStateOpen)
			bs.OpenRow = uint64(i)
			bs.CyclesToCmdAvailable = [numCmdKind]int{}
		}

		// Add 4 write commands (hits high watermark of 4)
		for i := range 4 {
			st.CommandQueues.Entries = append(
				st.CommandQueues.Entries,
				queueEntry{
					QueueIndex: 0,
					Command: commandState{
						ID:       ids.NewID(),
						Kind:     int(cmdKindWritePrecharge),
						Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i), Row: uint64(i)},
					},
					IsWrite: true,
				},
			)
		}

		require.False(t, st.CommandQueues.WriteDrainMode)

		// getCommandToIssue should trigger drain mode
		result := getCommandToIssue(spec, st)
		require.NotNil(t, result)
		require.True(t, st.CommandQueues.WriteDrainMode)
	})

	t.Run("should exit drain mode at low watermark", func(t *testing.T) {
		setup5(t)

		// Start in drain mode with exactly 2 writes (the low watermark)
		st.CommandQueues.WriteDrainMode = true

		bs0 := findBankState(&st.BankStates, 0, 0, 0)
		bs0.State = int(bankStateOpen)
		bs0.OpenRow = 1
		bs0.CyclesToCmdAvailable = [numCmdKind]int{}

		bs1 := findBankState(&st.BankStates, 0, 0, 1)
		bs1.State = int(bankStateOpen)
		bs1.OpenRow = 2
		bs1.CyclesToCmdAvailable = [numCmdKind]int{}

		st.CommandQueues.Entries = []queueEntry{
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       ids.NewID(),
					Kind:     int(cmdKindWritePrecharge),
					Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
				},
				IsWrite: true,
			},
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       ids.NewID(),
					Kind:     int(cmdKindWritePrecharge),
					Location: location{Rank: 0, BankGroup: 0, Bank: 1, Row: 2},
				},
				IsWrite: true,
			},
		}

		// 2 writes == low watermark, so drain mode should be exited
		result := getCommandToIssue(spec, st)
		require.NotNil(t, result)
		require.False(t, st.CommandQueues.WriteDrainMode)
	})

	t.Run("should respect separate read/write queue capacities", func(t *testing.T) {
		setup5(t)

		// Fill write queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       ids.NewID(),
				Kind:     int(cmdKindWritePrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			require.True(t, canAcceptCommand(st, cmd, spec))
			acceptCommand(st, cmd)
		}

		// One more write should be rejected
		extraWrite := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.False(t, canAcceptCommand(st, extraWrite, spec))

		// But a read should still be accepted
		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.True(t, canAcceptCommand(st, readCmd, spec))
	})

	t.Run("should respect separate read queue capacity", func(t *testing.T) {
		setup5(t)

		// Fill read queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       ids.NewID(),
				Kind:     int(cmdKindReadPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			require.True(t, canAcceptCommand(st, cmd, spec))
			acceptCommand(st, cmd)
		}

		// One more read should be rejected
		extraRead := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.False(t, canAcceptCommand(st, extraRead, spec))

		// But a write should still be accepted
		writeCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.True(t, canAcceptCommand(st, writeCmd, spec))
	})

	t.Run("should fall back to unified capacity when sizes are 0", func(t *testing.T) {
		setup5(t)

		spec.ReadQueueSize = 0
		spec.WriteQueueSize = 0
		spec.CommandQueueCapacity = 4

		// Fill unified queue to capacity
		for range 4 {
			cmd := &commandState{
				ID:       ids.NewID(),
				Kind:     int(cmdKindReadPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0},
			}
			require.True(t, canAcceptCommand(st, cmd, spec))
			acceptCommand(st, cmd)
		}

		// Both reads and writes should be rejected
		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.False(t, canAcceptCommand(st, readCmd, spec))

		writeCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		require.False(t, canAcceptCommand(st, writeCmd, spec))
	})

	t.Run("should identify write commands correctly", func(t *testing.T) {
		setup5(t)

		writeCmd := &commandState{Kind: int(cmdKindWrite)}
		require.True(t, isWriteCommand(writeCmd))

		writePCmd := &commandState{Kind: int(cmdKindWritePrecharge)}
		require.True(t, isWriteCommand(writePCmd))

		readCmd := &commandState{Kind: int(cmdKindRead)}
		require.False(t, isWriteCommand(readCmd))

		readPCmd := &commandState{Kind: int(cmdKindReadPrecharge)}
		require.False(t, isWriteCommand(readPCmd))

		actCmd := &commandState{Kind: int(cmdKindActivate)}
		require.False(t, isWriteCommand(actCmd))
	})

	t.Run("should tag queue entries with IsWrite flag", func(t *testing.T) {
		setup5(t)

		writeCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(st, writeCmd)
		require.True(t, st.CommandQueues.Entries[0].IsWrite)

		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(st, readCmd)
		require.False(t, st.CommandQueues.Entries[1].IsWrite)
	})
}

func TestBuilderConfiguration(t *testing.T) {
	t.Run("should set page policy via builder", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.PagePolicy = PagePolicyOpen
		ctrl := buildDRAM(sim, spec, "OpenPageCtrl", 16)

		builtSpec := ctrl.Spec
		require.Equal(t, PagePolicyOpen, builtSpec.PagePolicy)
	})

	t.Run("should set R/W queue sizes via builder", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.ReadQueueSize = 8
		spec.WriteQueueSize = 8
		spec.WriteHighWatermark = 6
		spec.WriteLowWatermark = 2
		ctrl := buildDRAM(sim, spec, "RWQueueCtrl", 16)

		builtSpec := ctrl.Spec
		require.Equal(t, 8, builtSpec.ReadQueueSize)
		require.Equal(t, 8, builtSpec.WriteQueueSize)
		require.Equal(t, 6, builtSpec.WriteHighWatermark)
		require.Equal(t, 2, builtSpec.WriteLowWatermark)
	})

	t.Run("should build with spec and preserve page policy", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		specWithOpenPage := DDR4Spec()
		specWithOpenPage.PagePolicy = PagePolicyOpen
		ctrl := buildDRAM(sim, specWithOpenPage, "DDR4OpenPage", 16)

		builtSpec := ctrl.Spec
		require.Equal(t, PagePolicyOpen, builtSpec.PagePolicy)
		require.Equal(t, 8, builtSpec.BurstLength)
	})

	t.Run("should build with spec and preserve R/W queue config", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		specWithRW := DDR4Spec()
		specWithRW.ReadQueueSize = 16
		specWithRW.WriteQueueSize = 16
		specWithRW.WriteHighWatermark = 12
		specWithRW.WriteLowWatermark = 4
		ctrl := buildDRAM(sim, specWithRW, "DDR4RWQueue", 16)

		builtSpec := ctrl.Spec
		require.Equal(t, 16, builtSpec.ReadQueueSize)
		require.Equal(t, 16, builtSpec.WriteQueueSize)
		require.Equal(t, 12, builtSpec.WriteHighWatermark)
		require.Equal(t, 4, builtSpec.WriteLowWatermark)
	})

	t.Run("should require a storage", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()

		require.PanicsWithValue(t, "dram: Resources.Storage is required", func() {
			builtComponent := modeling.MakeBuilder[*Comp](DefaultSpec()).
				WithSimulation(sim).
				Build("NoStorage")
			builtComponentPorts := defaultPorts(sim, "NoStorage", 16)
			builtComponent.BindPort("Top", builtComponentPorts.Top)
			builtComponent.BindPort("Control", builtComponentPorts.Control)
			if err := sim.Initialize(); err != nil {
				panic(err)
			}
		})
	})
}
