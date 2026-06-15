package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Address Operations", func() {
	It("should map address", func() {
		b := MakeBuilder()
		spec := b.buildSpec()

		loc := mapAddress(&spec, 0)
		Expect(loc.Channel).To(Equal(uint64(0)))
		Expect(loc.Rank).To(Equal(uint64(0)))
	})
})

var _ = Describe("Transaction Splitting", func() {
	It("should split a transaction into sub-transactions", func() {
		spec := &Spec{Log2AccessUnitSize: 6} // 64 bytes
		trans := &transactionState{
			HasRead: true,
			ReadMsg: memprotocol.ReadReq{},
		}
		trans.ReadMsg.Address = 0x100
		trans.ReadMsg.AccessByteSize = 128

		splitTransaction(spec, trans)
		// 128 bytes at 64-byte units = 2 sub-transactions
		Expect(trans.SubTransactions).To(HaveLen(2))
		Expect(trans.SubTransactions[0].Address).To(Equal(uint64(0x100)))
		Expect(trans.SubTransactions[1].Address).To(Equal(uint64(0x140)))
	})

	It("should align to unit boundaries", func() {
		spec := &Spec{Log2AccessUnitSize: 6} // 64 bytes
		trans := &transactionState{
			HasRead: true,
			ReadMsg: memprotocol.ReadReq{},
		}
		trans.ReadMsg.Address = 0x110 // Not aligned
		trans.ReadMsg.AccessByteSize = 4

		splitTransaction(spec, trans)
		Expect(trans.SubTransactions).To(HaveLen(1))
		Expect(trans.SubTransactions[0].Address).To(Equal(uint64(0x100)))
	})
})

var _ = Describe("Bank Operations", func() {
	It("should get required command kind for closed bank", func() {
		bs := &bankState{
			State:                int(bankStateClosed),
			CyclesToCmdAvailable: [numCmdKind]int{},
		}
		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		Expect(requiredKind).To(Equal(cmdKindActivate))
	})

	It("should get required command kind for open bank - same row", func() {
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
		Expect(requiredKind).To(Equal(cmdKindReadPrecharge))
	})

	It("should get precharge for open bank - different row", func() {
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
		Expect(requiredKind).To(Equal(cmdKindPrecharge))
	})

	It("should tick banks and count down timing gaps", func() {
		state := &State{
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

		progress := tickBanks(state)
		Expect(progress).To(BeTrue())
		bs := &state.BankStates.Entries[0].Data
		Expect(bs.CyclesToCmdAvailable[cmdKindRead]).To(Equal(2))
	})

	It("should complete pending reads/writes and mark subtrans done", func() {
		state := &State{
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

		completed := processPendingCompletions(state)

		Expect(completed).NotTo(BeEmpty())
		Expect(state.PendingCompletions).To(BeEmpty())
		Expect(state.Transactions[0].SubTransactions[0].Completed).To(BeTrue())
	})
})

var _ = Describe("Queue Operations", func() {
	It("should check if sub-trans queue can push", func() {
		state := &State{
			SubTransQueue: subTransQueueState{
				Entries: make([]subTransRef, 5),
			},
		}
		Expect(canPushSubTrans(state, 3, 10)).To(BeTrue())
		Expect(canPushSubTrans(state, 6, 10)).To(BeFalse())
	})

	It("should push sub-transactions", func() {
		state := &State{
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

		pushSubTrans(state, 0)
		Expect(state.SubTransQueue.Entries).To(HaveLen(2))
		Expect(state.SubTransQueue.Entries[0]).To(Equal(
			subTransRef{TxID: 0, SubIndex: 0}))
	})
})

var _ = Describe("DRAM Integration", func() {
	var (
		engine  timing.Engine
		memCtrl *modeling.Component[Spec, State, Resources]
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)
		memCtrl = MakeBuilder().
			WithRegistrar(reg).
			Build("MemCtrl")

		for _, name := range []string{"Top", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(memCtrl).
				WithSpec(modeling.PortSpec{BufSize: 1024}).
				Build(name)
			memCtrl.AssignPort(name, p)
		}
	})

	It("should read and write via direct connection", func() {
		srcPort := messaging.NewPort(nil, 1024, 1024, "Src.Top")
		conn := directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")
		topPort := memCtrl.GetPortByName("Top")
		conn.PlugIn(topPort)
		conn.PlugIn(srcPort)

		writeData := []byte{1, 2, 3, 4}
		write := memprotocol.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Address = 0x40
		write.Data = writeData
		write.Src = srcPort.AsRemote()
		write.Dst = topPort.AsRemote()
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "memprotocol.WriteReq"

		read := memprotocol.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Address = 0x40
		read.AccessByteSize = 4
		read.Src = srcPort.AsRemote()
		read.Dst = topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"

		srcPort.Send(write)
		srcPort.Send(read)

		engine.Run()

		// Collect responses
		var writeDone memprotocol.WriteDoneRsp
		var dataReady memprotocol.DataReadyRsp
		var gotWriteDone, gotDataReady bool

		for {
			msg := srcPort.RetrieveIncoming()
			if msg == nil {
				break
			}
			switch m := msg.(type) {
			case memprotocol.WriteDoneRsp:
				writeDone = m
				gotWriteDone = true
			case memprotocol.DataReadyRsp:
				dataReady = m
				gotDataReady = true
			}
		}

		Expect(gotWriteDone).To(BeTrue())
		Expect(writeDone.RspTo).To(Equal(write.ID))
		Expect(gotDataReady).To(BeTrue())
		Expect(dataReady.RspTo).To(Equal(read.ID))
		Expect(dataReady.Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

// ========================================================================
// New feature tests below
// ========================================================================

var _ = Describe("Predefined Specs", func() {
	It("should build with DDR4 spec", func() {
		engine := timing.NewSerialEngine()
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(DDR4Spec).
			Build("DDR4Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec()
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.NumRank).To(Equal(1))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(protoDDR4)))
	})

	It("should build with DDR5 spec", func() {
		engine := timing.NewSerialEngine()
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(DDR5Spec).
			Build("DDR5Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec()
		Expect(spec.BurstLength).To(Equal(16))
		Expect(spec.NumBankGroup).To(Equal(8))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(32))
		Expect(spec.Protocol).To(Equal(int(protoDDR5)))
	})

	It("should build with HBM2 spec", func() {
		engine := timing.NewSerialEngine()
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(HBM2Spec).
			Build("HBM2Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec()
		Expect(spec.BurstLength).To(Equal(4))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(128))
		Expect(spec.DeviceWidth).To(Equal(128))
		Expect(spec.Protocol).To(Equal(int(protoHBM2)))
	})

	It("should build with HBM3 spec", func() {
		engine := timing.NewSerialEngine()
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(HBM3Spec).
			Build("HBM3Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec()
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.DeviceWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(protoHBM3)))
	})

	It("should build with GDDR6 spec", func() {
		engine := timing.NewSerialEngine()
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(GDDR6Spec).
			Build("GDDR6Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec()
		Expect(spec.BurstLength).To(Equal(16))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(32))
		Expect(spec.DeviceWidth).To(Equal(32))
		Expect(spec.Protocol).To(Equal(int(protoGDDR6)))
	})

	It("should preserve DDR4 timing parameters", func() {
		Expect(DDR4Spec.TCL).To(Equal(16))
		Expect(DDR4Spec.TRCD).To(Equal(16))
		Expect(DDR4Spec.TRP).To(Equal(16))
		Expect(DDR4Spec.TRAS).To(Equal(39))
	})

	It("should preserve DDR5 timing parameters", func() {
		Expect(DDR5Spec.TCL).To(Equal(40))
		Expect(DDR5Spec.TRCD).To(Equal(40))
		Expect(DDR5Spec.TRP).To(Equal(40))
		Expect(DDR5Spec.TRAS).To(Equal(76))
	})

	It("should preserve HBM3 timing parameters", func() {
		Expect(HBM3Spec.TCL).To(Equal(36))
		Expect(HBM3Spec.TRCD).To(Equal(36))
		Expect(HBM3Spec.TRP).To(Equal(36))
		Expect(HBM3Spec.TRAS).To(Equal(72))
		Expect(HBM3Spec.TRCDRD).To(Equal(36))
		Expect(HBM3Spec.TRCDWR).To(Equal(24))
	})
})

var _ = Describe("Protocol Enums", func() {
	It("should have new protocol values after HMC", func() {
		Expect(int(protoDDR5)).To(BeNumerically(">", int(protoHMC)))
		Expect(int(protoHBM3)).To(BeNumerically(">", int(protoDDR5)))
		Expect(int(protoLPDDR5)).To(BeNumerically(">", int(protoHBM3)))
		Expect(int(protoHBM3E)).To(BeNumerically(">", int(protoLPDDR5)))
	})

	It("should recognize HBM3 as HBM", func() {
		Expect(protoHBM3.isHBM()).To(BeTrue())
	})

	It("should recognize HBM3E as HBM", func() {
		Expect(protoHBM3E.isHBM()).To(BeTrue())
	})

	It("should recognize HBM2 as HBM", func() {
		Expect(protoHBM2.isHBM()).To(BeTrue())
	})

	It("should recognize HBM as HBM", func() {
		Expect(protoHBM.isHBM()).To(BeTrue())
	})

	It("should not recognize DDR4 as HBM", func() {
		Expect(protoDDR4.isHBM()).To(BeFalse())
	})

	It("should not recognize DDR5 as HBM", func() {
		Expect(protoDDR5.isHBM()).To(BeFalse())
	})

	It("should recognize GDDR6 as GDDR", func() {
		Expect(protoGDDR6.isGDDR()).To(BeTrue())
	})

	It("should recognize GDDR5 as GDDR", func() {
		Expect(protoGDDR5.isGDDR()).To(BeTrue())
	})

	It("should recognize GDDR5X as GDDR", func() {
		Expect(protoGDDR5X.isGDDR()).To(BeTrue())
	})

	It("should not recognize DDR4 as GDDR", func() {
		Expect(protoDDR4.isGDDR()).To(BeFalse())
	})

	It("should not recognize HBM3 as GDDR", func() {
		Expect(protoHBM3.isGDDR()).To(BeFalse())
	})

	It("should have all original protocols in correct order", func() {
		Expect(int(protoDDR3)).To(Equal(0))
		Expect(int(protoDDR4)).To(Equal(1))
		Expect(int(protoGDDR5)).To(Equal(2))
		Expect(int(protoGDDR5X)).To(Equal(3))
		Expect(int(protoGDDR6)).To(Equal(4))
		Expect(int(protoLPDDR)).To(Equal(5))
		Expect(int(protoLPDDR3)).To(Equal(6))
		Expect(int(protoLPDDR4)).To(Equal(7))
		Expect(int(protoHBM)).To(Equal(8))
		Expect(int(protoHBM2)).To(Equal(9))
		Expect(int(protoHMC)).To(Equal(10))
	})
})

var _ = Describe("Open Page Policy", func() {
	var (
		spec  *Spec
		state *State
	)

	BeforeEach(func() {
		b := MakeBuilder()
		builtSpec := b.buildSpec()
		spec = &builtSpec
		state = &State{
			Transactions: []transactionState{
				{
					ID:      0,
					HasRead: true,
					ReadMsg: memprotocol.ReadReq{},
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
					WriteMsg: memprotocol.WriteReq{},
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
	})

	It("should create open page command with Read kind", func() {
		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createOpenPageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindRead)))
	})

	It("should create open page command with Write kind", func() {
		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createOpenPageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindWrite)))
	})

	It("should create close page command with ReadPrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createClosePageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindReadPrecharge)))
	})

	It("should create close page command with WritePrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createClosePageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindWritePrecharge)))
	})

	It("should keep bank open after Read command in open-page mode", func() {
		// Set up a bank that is Open at row 5
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindRead),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindRead: 10,
		}

		startCommand(cmdCycles, state, bs, cmd)

		// Bank should remain open
		Expect(bankStateKind(bs.State)).To(Equal(bankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(5)))
	})

	It("should keep bank open after Write command in open-page mode", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 7

		cmd := &commandState{
			Kind:     int(cmdKindWrite),
			Location: location{Row: 7, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindWrite: 10,
		}

		startCommand(cmdCycles, state, bs, cmd)

		// Bank should remain open
		Expect(bankStateKind(bs.State)).To(Equal(bankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(7)))
	})

	It("should close bank after ReadPrecharge command", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindReadPrecharge: 10,
		}

		startCommand(cmdCycles, state, bs, cmd)

		// Bank should be closed
		Expect(bankStateKind(bs.State)).To(Equal(bankStateClosed))
	})

	It("should close bank after WritePrecharge command", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(bankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		cmdCycles := map[commandKind]int{
			cmdKindWritePrecharge: 10,
		}

		startCommand(cmdCycles, state, bs, cmd)

		// Bank should be closed
		Expect(bankStateKind(bs.State)).To(Equal(bankStateClosed))
	})

	It("should select command creation based on page policy in tickSubTransQueue", func() {
		// Test with open-page policy
		spec.PagePolicy = PagePolicyOpen
		state.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindRead (not ReadPrecharge)
		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindRead)))
	})

	It("should use close-page commands when PagePolicyClose in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		state.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindReadPrecharge
		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindReadPrecharge)))
	})

	It("should use Write for write transaction with open-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyOpen
		state.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindWrite)))
	})

	It("should use WritePrecharge for write transaction with close-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		state.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindWritePrecharge)))
	})
})

var _ = Describe("FR-FCFS Scheduling", func() {
	var (
		spec  *Spec
		state *State
	)

	BeforeEach(func() {
		spec = &Spec{
			NumRank:              1,
			NumBankGroup:         1,
			NumBank:              2,
			CommandQueueCapacity: 16,
		}
		state = &State{
			CommandQueues: commandQueueState{
				NumQueues: 1,
				Entries:   []queueEntry{},
			},
			BankStates: initBankStatesFlat(1, 1, 2),
		}
	})

	It("should prioritize row-buffer hits over misses", func() {
		// Open bank 0 at row 5
		bs0 := findBankState(&state.BankStates, 0, 0, 0)
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
		state.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmdA},
			{QueueIndex: 0, Command: cmdB},
		}

		result := getCommandToIssue(spec, state)
		Expect(result).NotTo(BeNil())
		// Should pick the hit (row 5) even though it was added second
		Expect(result.Location.Row).To(Equal(uint64(5)))
	})

	It("should use FCFS when no row-buffer hits", func() {
		// Both banks closed — no row-buffer hits possible
		bs0 := findBankState(&state.BankStates, 0, 0, 0)
		bs0.CyclesToCmdAvailable = [numCmdKind]int{}
		bs1 := findBankState(&state.BankStates, 0, 0, 1)
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

		state.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmdA},
			{QueueIndex: 0, Command: cmdB},
		}

		result := getCommandToIssue(spec, state)
		Expect(result).NotTo(BeNil())
		// For closed banks, getRequiredCommandKind returns Activate.
		// The ready command should be an Activate for the older command.
		Expect(result.Kind).To(Equal(int(cmdKindActivate)))
		Expect(result.Location.Row).To(Equal(uint64(10)))
	})

	It("should handle empty queue", func() {
		state.CommandQueues.Entries = []queueEntry{}

		result := getCommandToIssue(spec, state)
		Expect(result).To(BeNil())
	})

	It("should return nil when all commands have timing constraints", func() {
		bs0 := findBankState(&state.BankStates, 0, 0, 0)
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
		state.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: cmd},
		}

		result := getCommandToIssue(spec, state)
		Expect(result).To(BeNil())
	})
})

var _ = Describe("Read/Write Queue Separation", func() {
	var (
		spec  *Spec
		state *State
	)

	BeforeEach(func() {
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
		state = &State{
			CommandQueues: commandQueueState{
				NumQueues: 1,
				Entries:   []queueEntry{},
			},
			BankStates: initBankStatesFlat(1, 1, 4),
		}
	})

	It("should count write commands", func() {
		state.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWritePrecharge)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
		}

		count := countWriteCommands(state)
		Expect(count).To(Equal(3))
	})

	It("should count zero writes in empty queue", func() {
		count := countWriteCommands(state)
		Expect(count).To(Equal(0))
	})

	It("should enter drain mode at high watermark", func() {
		// Set up 4 open banks, each with a write command that's a row hit
		for i := range 4 {
			bs := findBankState(&state.BankStates, 0, 0, i)
			bs.State = int(bankStateOpen)
			bs.OpenRow = uint64(i)
			bs.CyclesToCmdAvailable = [numCmdKind]int{}
		}

		// Add 4 write commands (hits high watermark of 4)
		for i := range 4 {
			state.CommandQueues.Entries = append(
				state.CommandQueues.Entries,
				queueEntry{
					QueueIndex: 0,
					Command: commandState{
						ID:       timing.GetIDGenerator().Generate(),
						Kind:     int(cmdKindWritePrecharge),
						Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i), Row: uint64(i)},
					},
					IsWrite: true,
				},
			)
		}

		Expect(state.CommandQueues.WriteDrainMode).To(BeFalse())

		// getCommandToIssue should trigger drain mode
		result := getCommandToIssue(spec, state)
		Expect(result).NotTo(BeNil())
		Expect(state.CommandQueues.WriteDrainMode).To(BeTrue())
	})

	It("should exit drain mode at low watermark", func() {
		// Start in drain mode with exactly 2 writes (the low watermark)
		state.CommandQueues.WriteDrainMode = true

		bs0 := findBankState(&state.BankStates, 0, 0, 0)
		bs0.State = int(bankStateOpen)
		bs0.OpenRow = 1
		bs0.CyclesToCmdAvailable = [numCmdKind]int{}

		bs1 := findBankState(&state.BankStates, 0, 0, 1)
		bs1.State = int(bankStateOpen)
		bs1.OpenRow = 2
		bs1.CyclesToCmdAvailable = [numCmdKind]int{}

		state.CommandQueues.Entries = []queueEntry{
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       timing.GetIDGenerator().Generate(),
					Kind:     int(cmdKindWritePrecharge),
					Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
				},
				IsWrite: true,
			},
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       timing.GetIDGenerator().Generate(),
					Kind:     int(cmdKindWritePrecharge),
					Location: location{Rank: 0, BankGroup: 0, Bank: 1, Row: 2},
				},
				IsWrite: true,
			},
		}

		// 2 writes == low watermark, so drain mode should be exited
		result := getCommandToIssue(spec, state)
		Expect(result).NotTo(BeNil())
		Expect(state.CommandQueues.WriteDrainMode).To(BeFalse())
	})

	It("should respect separate read/write queue capacities", func() {
		// Fill write queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       timing.GetIDGenerator().Generate(),
				Kind:     int(cmdKindWritePrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// One more write should be rejected
		extraWrite := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, extraWrite, spec)).To(BeFalse())

		// But a read should still be accepted
		readCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, readCmd, spec)).To(BeTrue())
	})

	It("should respect separate read queue capacity", func() {
		// Fill read queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       timing.GetIDGenerator().Generate(),
				Kind:     int(cmdKindReadPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// One more read should be rejected
		extraRead := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, extraRead, spec)).To(BeFalse())

		// But a write should still be accepted
		writeCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, writeCmd, spec)).To(BeTrue())
	})

	It("should fall back to unified capacity when sizes are 0", func() {
		spec.ReadQueueSize = 0
		spec.WriteQueueSize = 0
		spec.CommandQueueCapacity = 4

		// Fill unified queue to capacity
		for range 4 {
			cmd := &commandState{
				ID:       timing.GetIDGenerator().Generate(),
				Kind:     int(cmdKindReadPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: 0},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// Both reads and writes should be rejected
		readCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, readCmd, spec)).To(BeFalse())

		writeCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, writeCmd, spec)).To(BeFalse())
	})

	It("should identify write commands correctly", func() {
		writeCmd := &commandState{Kind: int(cmdKindWrite)}
		Expect(isWriteCommand(writeCmd)).To(BeTrue())

		writePCmd := &commandState{Kind: int(cmdKindWritePrecharge)}
		Expect(isWriteCommand(writePCmd)).To(BeTrue())

		readCmd := &commandState{Kind: int(cmdKindRead)}
		Expect(isWriteCommand(readCmd)).To(BeFalse())

		readPCmd := &commandState{Kind: int(cmdKindReadPrecharge)}
		Expect(isWriteCommand(readPCmd)).To(BeFalse())

		actCmd := &commandState{Kind: int(cmdKindActivate)}
		Expect(isWriteCommand(actCmd)).To(BeFalse())
	})

	It("should tag queue entries with IsWrite flag", func() {
		writeCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(state, writeCmd)
		Expect(state.CommandQueues.Entries[0].IsWrite).To(BeTrue())

		readCmd := &commandState{
			ID:       timing.GetIDGenerator().Generate(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(state, readCmd)
		Expect(state.CommandQueues.Entries[1].IsWrite).To(BeFalse())
	})
})

var _ = Describe("Builder Configuration", func() {
	It("should set page policy via builder", func() {
		engine := timing.NewSerialEngine()
		spec := DefaultSpec()
		spec.PagePolicy = PagePolicyOpen
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("OpenPageCtrl")

		builtSpec := ctrl.Spec()
		Expect(builtSpec.PagePolicy).To(Equal(PagePolicyOpen))
	})

	It("should set R/W queue sizes via builder", func() {
		engine := timing.NewSerialEngine()
		spec := DefaultSpec()
		spec.ReadQueueSize = 8
		spec.WriteQueueSize = 8
		spec.WriteHighWatermark = 6
		spec.WriteLowWatermark = 2
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("RWQueueCtrl")

		builtSpec := ctrl.Spec()
		Expect(builtSpec.ReadQueueSize).To(Equal(8))
		Expect(builtSpec.WriteQueueSize).To(Equal(8))
		Expect(builtSpec.WriteHighWatermark).To(Equal(6))
		Expect(builtSpec.WriteLowWatermark).To(Equal(2))
	})

	It("should build with spec and preserve page policy", func() {
		engine := timing.NewSerialEngine()
		specWithOpenPage := DDR4Spec
		specWithOpenPage.PagePolicy = PagePolicyOpen
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(specWithOpenPage).
			Build("DDR4OpenPage")

		builtSpec := ctrl.Spec()
		Expect(builtSpec.PagePolicy).To(Equal(PagePolicyOpen))
		Expect(builtSpec.BurstLength).To(Equal(8))
	})

	It("should build with spec and preserve R/W queue config", func() {
		engine := timing.NewSerialEngine()
		specWithRW := DDR4Spec
		specWithRW.ReadQueueSize = 16
		specWithRW.WriteQueueSize = 16
		specWithRW.WriteHighWatermark = 12
		specWithRW.WriteLowWatermark = 4
		ctrl := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(specWithRW).
			Build("DDR4RWQueue")

		builtSpec := ctrl.Spec()
		Expect(builtSpec.ReadQueueSize).To(Equal(16))
		Expect(builtSpec.WriteQueueSize).To(Equal(16))
		Expect(builtSpec.WriteHighWatermark).To(Equal(12))
		Expect(builtSpec.WriteLowWatermark).To(Equal(4))
	})
})
