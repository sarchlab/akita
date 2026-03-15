package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
)

var _ = Describe("Address Operations", func() {
	It("should convert external to internal without converter", func() {
		spec := &Spec{HasAddrConverter: false}
		Expect(convertExternalToInternal(spec, 0x1000)).To(
			Equal(uint64(0x1000)))
	})

	It("should convert external to internal with interleaving", func() {
		spec := &Spec{
			HasAddrConverter:    true,
			InterleavingSize:    4096,
			TotalNumOfElements:  8,
			CurrentElementIndex: 3,
			Offset:              0,
		}
		// addr = 0 + highBits*8*4096 + 3*4096 + lowBits
		// For addr = 3*4096 = 12288: highBits=0, lowBits=0
		// internal = 0*4096 + 0 = 0
		Expect(convertExternalToInternal(spec, 3*4096)).To(
			Equal(uint64(0)))
	})

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
			ReadMsg: mem.ReadReq{},
		}
		trans.ReadMsg.Address = 0x100
		trans.ReadMsg.AccessByteSize = 128

		splitTransaction(spec, trans, 0)
		// 128 bytes at 64-byte units = 2 sub-transactions
		Expect(trans.SubTransactions).To(HaveLen(2))
		Expect(trans.SubTransactions[0].Address).To(Equal(uint64(0x100)))
		Expect(trans.SubTransactions[1].Address).To(Equal(uint64(0x140)))
	})

	It("should align to unit boundaries", func() {
		spec := &Spec{Log2AccessUnitSize: 6} // 64 bytes
		trans := &transactionState{
			HasRead: true,
			ReadMsg: mem.ReadReq{},
		}
		trans.ReadMsg.Address = 0x110 // Not aligned
		trans.ReadMsg.AccessByteSize = 4

		splitTransaction(spec, trans, 0)
		Expect(trans.SubTransactions).To(HaveLen(1))
		Expect(trans.SubTransactions[0].Address).To(Equal(uint64(0x100)))
	})
})

var _ = Describe("Bank Operations", func() {
	It("should get required command kind for closed bank", func() {
		bs := &bankState{
			State:                int(BankStateClosed),
			CyclesToCmdAvailable: make(map[string]int),
		}
		cmd := &commandState{
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		Expect(requiredKind).To(Equal(CmdKindActivate))
	})

	It("should get required command kind for open bank - same row", func() {
		bs := &bankState{
			State:                int(BankStateOpen),
			OpenRow:              10,
			CyclesToCmdAvailable: make(map[string]int),
		}
		cmd := &commandState{
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		Expect(requiredKind).To(Equal(CmdKindReadPrecharge))
	})

	It("should get precharge for open bank - different row", func() {
		bs := &bankState{
			State:                int(BankStateOpen),
			OpenRow:              5,
			CyclesToCmdAvailable: make(map[string]int),
		}
		cmd := &commandState{
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Row: 10},
		}

		requiredKind := getRequiredCommandKind(bs, cmd)
		Expect(requiredKind).To(Equal(CmdKindPrecharge))
	})

	It("should tick banks and count down", func() {
		spec := &Spec{
			CmdCycles: map[CommandKind]int{
				CmdKindActivate: 5,
			},
		}
		state := &State{
			BankStates: bankStatesFlat{
				NumRanks:      1,
				NumBankGroups: 1,
				NumBanks:      1,
				Entries: []bankEntry{
					{
						Rank: 0, BankGroup: 0, BankIndex: 0,
						Data: bankState{
							State:         int(BankStateClosed),
							HasCurrentCmd: true,
							CurrentCmd: commandState{
								Kind:      int(CmdKindActivate),
								CycleLeft: 2,
							},
							CyclesToCmdAvailable: map[string]int{
								cmdKindToString(CmdKindRead): 3,
							},
						},
					},
				},
			},
		}

		progress := tickBanks(spec, state)
		Expect(progress).To(BeTrue())
		bs := &state.BankStates.Entries[0].Data
		Expect(bs.CurrentCmd.CycleLeft).To(Equal(1))
		Expect(bs.CyclesToCmdAvailable[cmdKindToString(CmdKindRead)]).To(
			Equal(2))
	})

	It("should complete command and mark subtrans done", func() {
		state := &State{
			Transactions: []transactionState{
				{
					HasRead: true,
					SubTransactions: []subTransState{
						{ID: "st1", Completed: false},
					},
				},
			},
			BankStates: bankStatesFlat{
				Entries: []bankEntry{
					{
						Data: bankState{
							State:         int(BankStateOpen),
							HasCurrentCmd: true,
							CurrentCmd: commandState{
								Kind:      int(CmdKindReadPrecharge),
								CycleLeft: 1,
								SubTransRef: subTransRef{
									TransIndex: 0,
									SubIndex:   0,
								},
							},
							CyclesToCmdAvailable: make(map[string]int),
						},
					},
				},
			},
		}

		spec := &Spec{CmdCycles: map[CommandKind]int{}}
		tickBanks(spec, state)

		Expect(state.BankStates.Entries[0].Data.HasCurrentCmd).To(BeFalse())
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
						{ID: "st0"},
						{ID: "st1"},
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
			subTransRef{TransIndex: 0, SubIndex: 0}))
	})
})

var _ = Describe("DRAM Integration", func() {
	var (
		engine  sim.Engine
		memCtrl *modeling.Component[Spec, State]
	)

	BeforeEach(func() {
		engine = sim.NewSerialEngine()
		memCtrl = MakeBuilder().
			WithEngine(engine).
			WithTopPort(sim.NewPort(nil, 1024, 1024, "MemCtrl.TopPort")).
			Build("MemCtrl")
	})

	It("should read and write via direct connection", func() {
		srcPort := sim.NewPort(nil, 1024, 1024, "SrcPort")
		conn := directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		topPort := memCtrl.GetPortByName("Top")
		conn.PlugIn(topPort)
		conn.PlugIn(srcPort)

		writeData := []byte{1, 2, 3, 4}
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x40
		write.Data = writeData
		write.Src = srcPort.AsRemote()
		write.Dst = topPort.AsRemote()
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "mem.WriteReq"

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x40
		read.AccessByteSize = 4
		read.Src = srcPort.AsRemote()
		read.Dst = topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		srcPort.Send(write)
		srcPort.Send(read)

		engine.Run()

		// Collect responses
		var writeDone *mem.WriteDoneRsp
		var dataReady *mem.DataReadyRsp

		for {
			msg := srcPort.RetrieveIncoming()
			if msg == nil {
				break
			}
			switch m := msg.(type) {
			case *mem.WriteDoneRsp:
				writeDone = m
			case *mem.DataReadyRsp:
				dataReady = m
			}
		}

		Expect(writeDone).NotTo(BeNil())
		Expect(writeDone.RspTo).To(Equal(write.ID))
		Expect(dataReady).NotTo(BeNil())
		Expect(dataReady.RspTo).To(Equal(read.ID))
		Expect(dataReady.Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

// ========================================================================
// New feature tests below
// ========================================================================

var _ = Describe("Predefined Specs", func() {
	It("should build with DDR4 spec", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(DDR4Spec).
			WithTopPort(port).
			Build("DDR4Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.GetSpec()
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.NumRank).To(Equal(1))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(DDR4)))
	})

	It("should build with DDR5 spec", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(DDR5Spec).
			WithTopPort(port).
			Build("DDR5Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.GetSpec()
		Expect(spec.BurstLength).To(Equal(16))
		Expect(spec.NumBankGroup).To(Equal(8))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(32))
		Expect(spec.Protocol).To(Equal(int(DDR5)))
	})

	It("should build with HBM2 spec", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(HBM2Spec).
			WithTopPort(port).
			Build("HBM2Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.GetSpec()
		Expect(spec.BurstLength).To(Equal(4))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(128))
		Expect(spec.DeviceWidth).To(Equal(128))
		Expect(spec.Protocol).To(Equal(int(HBM2)))
	})

	It("should build with HBM3 spec", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(HBM3Spec).
			WithTopPort(port).
			Build("HBM3Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.GetSpec()
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.DeviceWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(HBM3)))
	})

	It("should build with GDDR6 spec", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(GDDR6Spec).
			WithTopPort(port).
			Build("GDDR6Ctrl")
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.GetSpec()
		Expect(spec.BurstLength).To(Equal(16))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(32))
		Expect(spec.DeviceWidth).To(Equal(32))
		Expect(spec.Protocol).To(Equal(int(GDDR6)))
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
		Expect(int(DDR5)).To(BeNumerically(">", int(HMC)))
		Expect(int(HBM3)).To(BeNumerically(">", int(DDR5)))
		Expect(int(LPDDR5)).To(BeNumerically(">", int(HBM3)))
		Expect(int(HBM3E)).To(BeNumerically(">", int(LPDDR5)))
	})

	It("should recognize HBM3 as HBM", func() {
		Expect(HBM3.isHBM()).To(BeTrue())
	})

	It("should recognize HBM3E as HBM", func() {
		Expect(HBM3E.isHBM()).To(BeTrue())
	})

	It("should recognize HBM2 as HBM", func() {
		Expect(HBM2.isHBM()).To(BeTrue())
	})

	It("should recognize HBM as HBM", func() {
		Expect(HBM.isHBM()).To(BeTrue())
	})

	It("should not recognize DDR4 as HBM", func() {
		Expect(DDR4.isHBM()).To(BeFalse())
	})

	It("should not recognize DDR5 as HBM", func() {
		Expect(DDR5.isHBM()).To(BeFalse())
	})

	It("should recognize GDDR6 as GDDR", func() {
		Expect(GDDR6.isGDDR()).To(BeTrue())
	})

	It("should recognize GDDR5 as GDDR", func() {
		Expect(GDDR5.isGDDR()).To(BeTrue())
	})

	It("should recognize GDDR5X as GDDR", func() {
		Expect(GDDR5X.isGDDR()).To(BeTrue())
	})

	It("should not recognize DDR4 as GDDR", func() {
		Expect(DDR4.isGDDR()).To(BeFalse())
	})

	It("should not recognize HBM3 as GDDR", func() {
		Expect(HBM3.isGDDR()).To(BeFalse())
	})

	It("should have all original protocols in correct order", func() {
		Expect(int(DDR3)).To(Equal(0))
		Expect(int(DDR4)).To(Equal(1))
		Expect(int(GDDR5)).To(Equal(2))
		Expect(int(GDDR5X)).To(Equal(3))
		Expect(int(GDDR6)).To(Equal(4))
		Expect(int(LPDDR)).To(Equal(5))
		Expect(int(LPDDR3)).To(Equal(6))
		Expect(int(LPDDR4)).To(Equal(7))
		Expect(int(HBM)).To(Equal(8))
		Expect(int(HBM2)).To(Equal(9))
		Expect(int(HMC)).To(Equal(10))
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
					HasRead: true,
					ReadMsg: mem.ReadReq{},
					SubTransactions: []subTransState{
						{
							ID:               "st-read-0",
							Address:          0x0,
							Completed:        false,
							TransactionIndex: 0,
						},
					},
				},
				{
					HasWrite: true,
					WriteMsg: mem.WriteReq{},
					SubTransactions: []subTransState{
						{
							ID:               "st-write-0",
							Address:          0x0,
							Completed:        false,
							TransactionIndex: 1,
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

		ref := subTransRef{TransIndex: 0, SubIndex: 0}
		cmd := createOpenPageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(CmdKindRead)))
	})

	It("should create open page command with Write kind", func() {
		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TransIndex: 1, SubIndex: 0}
		cmd := createOpenPageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(CmdKindWrite)))
	})

	It("should create close page command with ReadPrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TransIndex: 0, SubIndex: 0}
		cmd := createClosePageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(CmdKindReadPrecharge)))
	})

	It("should create close page command with WritePrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TransIndex: 1, SubIndex: 0}
		cmd := createClosePageCommand(spec, state, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(CmdKindWritePrecharge)))
	})

	It("should keep bank open after Read command in open-page mode", func() {
		// Set up a bank that is Open at row 5
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(BankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(CmdKindRead),
			Location: Location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		spec.CmdCycles = map[CommandKind]int{
			CmdKindRead: 10,
		}

		startCommand(spec, state, bs, cmd)

		// Bank should remain open
		Expect(BankStateKind(bs.State)).To(Equal(BankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(5)))
	})

	It("should keep bank open after Write command in open-page mode", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(BankStateOpen)
		bs.OpenRow = 7

		cmd := &commandState{
			Kind:     int(CmdKindWrite),
			Location: Location{Row: 7, Rank: 0, BankGroup: 0, Bank: 0},
		}

		spec.CmdCycles = map[CommandKind]int{
			CmdKindWrite: 10,
		}

		startCommand(spec, state, bs, cmd)

		// Bank should remain open
		Expect(BankStateKind(bs.State)).To(Equal(BankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(7)))
	})

	It("should close bank after ReadPrecharge command", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(BankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		spec.CmdCycles = map[CommandKind]int{
			CmdKindReadPrecharge: 10,
		}

		startCommand(spec, state, bs, cmd)

		// Bank should be closed
		Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))
	})

	It("should close bank after WritePrecharge command", func() {
		bs := &state.BankStates.Entries[0].Data
		bs.State = int(BankStateOpen)
		bs.OpenRow = 5

		cmd := &commandState{
			Kind:     int(CmdKindWritePrecharge),
			Location: Location{Row: 5, Rank: 0, BankGroup: 0, Bank: 0},
		}

		spec.CmdCycles = map[CommandKind]int{
			CmdKindWritePrecharge: 10,
		}

		startCommand(spec, state, bs, cmd)

		// Bank should be closed
		Expect(BankStateKind(bs.State)).To(Equal(BankStateClosed))
	})

	It("should select command creation based on page policy in tickSubTransQueue", func() {
		// Test with open-page policy
		spec.PagePolicy = PagePolicyOpen
		state.SubTransQueue.Entries = []subTransRef{
			{TransIndex: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindRead (not ReadPrecharge)
		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(CmdKindRead)))
	})

	It("should use close-page commands when PagePolicyClose in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		state.SubTransQueue.Entries = []subTransRef{
			{TransIndex: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindReadPrecharge
		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(CmdKindReadPrecharge)))
	})

	It("should use Write for write transaction with open-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyOpen
		state.SubTransQueue.Entries = []subTransRef{
			{TransIndex: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(CmdKindWrite)))
	})

	It("should use WritePrecharge for write transaction with close-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		state.SubTransQueue.Entries = []subTransRef{
			{TransIndex: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(spec, state)
		Expect(progress).To(BeTrue())

		Expect(state.CommandQueues.Entries).To(HaveLen(1))
		Expect(state.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(CmdKindWritePrecharge)))
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
			CmdCycles: map[CommandKind]int{
				CmdKindRead:          10,
				CmdKindReadPrecharge: 10,
				CmdKindActivate:      5,
				CmdKindPrecharge:     5,
			},
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
		bs0.State = int(BankStateOpen)
		bs0.OpenRow = 5
		bs0.CyclesToCmdAvailable = make(map[string]int)

		// Command A: targets row 10 on bank 0 (miss — needs precharge)
		cmdA := commandState{
			ID:   "cmd-miss",
			Kind: int(CmdKindReadPrecharge),
			Location: Location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 10,
			},
		}
		// Command B: targets row 5 on bank 0 (hit — matching row)
		cmdB := commandState{
			ID:   "cmd-hit",
			Kind: int(CmdKindReadPrecharge),
			Location: Location{
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
		bs0.CyclesToCmdAvailable = make(map[string]int)
		bs1 := findBankState(&state.BankStates, 0, 0, 1)
		bs1.CyclesToCmdAvailable = make(map[string]int)

		cmdA := commandState{
			ID:   "cmd-older",
			Kind: int(CmdKindReadPrecharge),
			Location: Location{
				Rank: 0, BankGroup: 0, Bank: 0, Row: 10,
			},
		}
		cmdB := commandState{
			ID:   "cmd-newer",
			Kind: int(CmdKindReadPrecharge),
			Location: Location{
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
		Expect(result.Kind).To(Equal(int(CmdKindActivate)))
		Expect(result.Location.Row).To(Equal(uint64(10)))
	})

	It("should handle empty queue", func() {
		state.CommandQueues.Entries = []queueEntry{}

		result := getCommandToIssue(spec, state)
		Expect(result).To(BeNil())
	})

	It("should return nil when all commands have timing constraints", func() {
		bs0 := findBankState(&state.BankStates, 0, 0, 0)
		bs0.CyclesToCmdAvailable = map[string]int{
			cmdKindToString(CmdKindActivate):      5,
			cmdKindToString(CmdKindReadPrecharge):  5,
			cmdKindToString(CmdKindRead):           5,
			cmdKindToString(CmdKindPrecharge):      5,
		}

		cmd := commandState{
			ID:   "cmd-blocked",
			Kind: int(CmdKindReadPrecharge),
			Location: Location{
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
			CmdCycles: map[CommandKind]int{
				CmdKindRead:           10,
				CmdKindReadPrecharge:  10,
				CmdKindWrite:          10,
				CmdKindWritePrecharge: 10,
				CmdKindActivate:       5,
				CmdKindPrecharge:      5,
			},
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
			{QueueIndex: 0, Command: commandState{Kind: int(CmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(CmdKindWrite)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(CmdKindWritePrecharge)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(CmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(CmdKindWrite)}, IsWrite: true},
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
		for i := 0; i < 4; i++ {
			bs := findBankState(&state.BankStates, 0, 0, i)
			bs.State = int(BankStateOpen)
			bs.OpenRow = uint64(i)
			bs.CyclesToCmdAvailable = make(map[string]int)
		}

		// Add 4 write commands (hits high watermark of 4)
		for i := 0; i < 4; i++ {
			state.CommandQueues.Entries = append(
				state.CommandQueues.Entries,
				queueEntry{
					QueueIndex: 0,
					Command: commandState{
						ID:       sim.GetIDGenerator().Generate(),
						Kind:     int(CmdKindWritePrecharge),
						Location: Location{Rank: 0, BankGroup: 0, Bank: uint64(i), Row: uint64(i)},
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
		bs0.State = int(BankStateOpen)
		bs0.OpenRow = 1
		bs0.CyclesToCmdAvailable = make(map[string]int)

		bs1 := findBankState(&state.BankStates, 0, 0, 1)
		bs1.State = int(BankStateOpen)
		bs1.OpenRow = 2
		bs1.CyclesToCmdAvailable = make(map[string]int)

		state.CommandQueues.Entries = []queueEntry{
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       sim.GetIDGenerator().Generate(),
					Kind:     int(CmdKindWritePrecharge),
					Location: Location{Rank: 0, BankGroup: 0, Bank: 0, Row: 1},
				},
				IsWrite: true,
			},
			{
				QueueIndex: 0,
				Command: commandState{
					ID:       sim.GetIDGenerator().Generate(),
					Kind:     int(CmdKindWritePrecharge),
					Location: Location{Rank: 0, BankGroup: 0, Bank: 1, Row: 2},
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
		for i := 0; i < 4; i++ {
			cmd := &commandState{
				ID:       sim.GetIDGenerator().Generate(),
				Kind:     int(CmdKindWritePrecharge),
				Location: Location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// One more write should be rejected
		extraWrite := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindWritePrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, extraWrite, spec)).To(BeFalse())

		// But a read should still be accepted
		readCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, readCmd, spec)).To(BeTrue())
	})

	It("should respect separate read queue capacity", func() {
		// Fill read queue to capacity (4)
		for i := 0; i < 4; i++ {
			cmd := &commandState{
				ID:       sim.GetIDGenerator().Generate(),
				Kind:     int(CmdKindReadPrecharge),
				Location: Location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// One more read should be rejected
		extraRead := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, extraRead, spec)).To(BeFalse())

		// But a write should still be accepted
		writeCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindWritePrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, writeCmd, spec)).To(BeTrue())
	})

	It("should fall back to unified capacity when sizes are 0", func() {
		spec.ReadQueueSize = 0
		spec.WriteQueueSize = 0
		spec.CommandQueueCapacity = 4

		// Fill unified queue to capacity
		for i := 0; i < 4; i++ {
			cmd := &commandState{
				ID:       sim.GetIDGenerator().Generate(),
				Kind:     int(CmdKindReadPrecharge),
				Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
			}
			Expect(canAcceptCommand(state, cmd, spec)).To(BeTrue())
			acceptCommand(state, cmd)
		}

		// Both reads and writes should be rejected
		readCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, readCmd, spec)).To(BeFalse())

		writeCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindWritePrecharge),
			Location: Location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(state, writeCmd, spec)).To(BeFalse())
	})

	It("should identify write commands correctly", func() {
		writeCmd := &commandState{Kind: int(CmdKindWrite)}
		Expect(isWriteCommand(writeCmd)).To(BeTrue())

		writePCmd := &commandState{Kind: int(CmdKindWritePrecharge)}
		Expect(isWriteCommand(writePCmd)).To(BeTrue())

		readCmd := &commandState{Kind: int(CmdKindRead)}
		Expect(isWriteCommand(readCmd)).To(BeFalse())

		readPCmd := &commandState{Kind: int(CmdKindReadPrecharge)}
		Expect(isWriteCommand(readPCmd)).To(BeFalse())

		actCmd := &commandState{Kind: int(CmdKindActivate)}
		Expect(isWriteCommand(actCmd)).To(BeFalse())
	})

	It("should tag queue entries with IsWrite flag", func() {
		writeCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindWritePrecharge),
			Location: Location{Rank: 0},
		}
		acceptCommand(state, writeCmd)
		Expect(state.CommandQueues.Entries[0].IsWrite).To(BeTrue())

		readCmd := &commandState{
			ID:       sim.GetIDGenerator().Generate(),
			Kind:     int(CmdKindReadPrecharge),
			Location: Location{Rank: 0},
		}
		acceptCommand(state, readCmd)
		Expect(state.CommandQueues.Entries[1].IsWrite).To(BeFalse())
	})
})

var _ = Describe("Builder Configuration", func() {
	It("should set page policy via builder", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithPagePolicy(PagePolicyOpen).
			WithTopPort(port).
			Build("OpenPageCtrl")

		spec := ctrl.GetSpec()
		Expect(spec.PagePolicy).To(Equal(PagePolicyOpen))
	})

	It("should set R/W queue sizes via builder", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithReadQueueSize(8).
			WithWriteQueueSize(8).
			WithWriteHighWatermark(6).
			WithWriteLowWatermark(2).
			WithTopPort(port).
			Build("RWQueueCtrl")

		spec := ctrl.GetSpec()
		Expect(spec.ReadQueueSize).To(Equal(8))
		Expect(spec.WriteQueueSize).To(Equal(8))
		Expect(spec.WriteHighWatermark).To(Equal(6))
		Expect(spec.WriteLowWatermark).To(Equal(2))
	})

	It("should build with spec and preserve page policy", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		specWithOpenPage := DDR4Spec
		specWithOpenPage.PagePolicy = PagePolicyOpen
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(specWithOpenPage).
			WithTopPort(port).
			Build("DDR4OpenPage")

		builtSpec := ctrl.GetSpec()
		Expect(builtSpec.PagePolicy).To(Equal(PagePolicyOpen))
		Expect(builtSpec.BurstLength).To(Equal(8))
	})

	It("should build with spec and preserve R/W queue config", func() {
		engine := sim.NewSerialEngine()
		port := sim.NewPort(nil, 1024, 1024, "TestPort")
		specWithRW := DDR4Spec
		specWithRW.ReadQueueSize = 16
		specWithRW.WriteQueueSize = 16
		specWithRW.WriteHighWatermark = 12
		specWithRW.WriteLowWatermark = 4
		ctrl := MakeBuilder().
			WithEngine(engine).
			WithSpec(specWithRW).
			WithTopPort(port).
			Build("DDR4RWQueue")

		builtSpec := ctrl.GetSpec()
		Expect(builtSpec.ReadQueueSize).To(Equal(16))
		Expect(builtSpec.WriteQueueSize).To(Equal(16))
		Expect(builtSpec.WriteHighWatermark).To(Equal(12))
		Expect(builtSpec.WriteLowWatermark).To(Equal(4))
	})
})
