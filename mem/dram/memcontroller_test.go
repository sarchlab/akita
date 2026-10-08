package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var _ = Describe("Address Operations", func() {
	It("should map address", func() {
		spec := Definition.DefaultSpec

		loc := newAddrMapping(&spec).mapAddress(0)
		Expect(loc.Channel).To(Equal(uint64(0)))
		Expect(loc.Rank).To(Equal(uint64(0)))
	})
})

var _ = Describe("Transaction Splitting", func() {
	var ids timing.Simulation
	BeforeEach(func() { ids = modeling.NewStandaloneSimulation(timing.NewSerialEngine()) })
	It("should split a transaction into sub-transactions", func() {
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
		Expect(trans.SubTransactions).To(HaveLen(2))
		Expect(trans.SubTransactions[0].Address).To(Equal(uint64(0x100)))
		Expect(trans.SubTransactions[1].Address).To(Equal(uint64(0x140)))
	})

	It("should align to unit boundaries", func() {
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
		Expect(progress).To(BeTrue())
		bs := &st.BankStates.Entries[0].Data
		Expect(bs.CyclesToCmdAvailable[cmdKindRead]).To(Equal(2))
	})

	It("should complete pending reads/writes and mark subtrans done", func() {
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

		Expect(completed).NotTo(BeEmpty())
		Expect(st.PendingCompletions).To(BeEmpty())
		Expect(st.Transactions[0].SubTransactions[0].Completed).To(BeTrue())
	})
})

var _ = Describe("Queue Operations", func() {
	It("should check if sub-trans queue can push", func() {
		st := &state{
			SubTransQueue: subTransQueueState{
				Entries: make([]subTransRef, 5),
			},
		}
		Expect(canPushSubTrans(st, 3, 10)).To(BeTrue())
		Expect(canPushSubTrans(st, 6, 10)).To(BeFalse())
	})

	It("should push sub-transactions", func() {
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
		Expect(st.SubTransQueue.Entries).To(HaveLen(2))
		Expect(st.SubTransQueue.Entries[0]).To(Equal(
			subTransRef{TxID: 0, SubIndex: 0}))
	})
})

var _ = Describe("DRAM Integration", func() {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		memCtrl *Comp
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		memCtrl = buildDRAM(sim, Definition.DefaultSpec, "MemCtrl", 1024)
	})

	It("should read and write via direct connection", func() {
		srcPort := newDriverPort("Src.Top", 1024)
		conn := direct.NewConnection("Conn", sim, timing.GHz)
		topPort := memCtrl.Ports.Top
		conn.PlugIn(topPort)
		conn.PlugIn(srcPort)

		writeData := []byte{1, 2, 3, 4}
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x40,
			Data:    writeData},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: len(writeData) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x40,
			AccessByteSize: 4},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		srcPort.Send(write)
		srcPort.Send(read)

		Expect(engine.Run()).To(Succeed())

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

		Expect(gotWriteDone).To(BeTrue())
		Expect(writeDone.RspTo).To(Equal(write.ID))
		Expect(gotDataReady).To(BeTrue())
		Expect(dataReady.RspTo).To(Equal(read.ID))
		Expect(dataReady.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{1, 2, 3, 4}))
	})
})

// ========================================================================
// New feature tests below
// ========================================================================

var _ = Describe("Predefined Specs", func() {
	It("should build with DDR4 spec", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		ctrl := buildDRAM(sim, DDR4Spec, "DDR4Ctrl", 16)
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.NumRank).To(Equal(1))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(protoDDR4)))
	})

	It("should build with DDR5 spec", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		ctrl := buildDRAM(sim, DDR5Spec, "DDR5Ctrl", 16)
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec
		Expect(spec.BurstLength).To(Equal(16))
		Expect(spec.NumBankGroup).To(Equal(8))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(32))
		Expect(spec.Protocol).To(Equal(int(protoDDR5)))
	})

	It("should build with HBM2 spec", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		ctrl := buildDRAM(sim, HBM2Spec, "HBM2Ctrl", 16)
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec
		Expect(spec.BurstLength).To(Equal(4))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.NumBank).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(128))
		Expect(spec.DeviceWidth).To(Equal(128))
		Expect(spec.Protocol).To(Equal(int(protoHBM2)))
	})

	It("should build with HBM3 spec", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		ctrl := buildDRAM(sim, HBM3Spec, "HBM3Ctrl", 16)
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec
		Expect(spec.BurstLength).To(Equal(8))
		Expect(spec.NumBankGroup).To(Equal(4))
		Expect(spec.BusWidth).To(Equal(64))
		Expect(spec.DeviceWidth).To(Equal(64))
		Expect(spec.Protocol).To(Equal(int(protoHBM3)))
	})

	It("should build with GDDR6 spec", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		ctrl := buildDRAM(sim, GDDR6Spec, "GDDR6Ctrl", 16)
		Expect(ctrl).NotTo(BeNil())
		spec := ctrl.Spec
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
		ids  timing.Simulation
		spec *Spec
		st   *state
	)

	BeforeEach(func() {
		ids = modeling.NewStandaloneSimulation(timing.NewSerialEngine())
		defaultSpec := Definition.DefaultSpec
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
	})

	It("should create open page command with Read kind", func() {
		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createOpenPageCommand(ids.NewID, spec, st, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindRead)))
	})

	It("should create open page command with Write kind", func() {
		spec.PagePolicy = PagePolicyOpen

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createOpenPageCommand(ids.NewID, spec, st, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindWrite)))
	})

	It("should create close page command with ReadPrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 0, SubIndex: 0}
		cmd := createClosePageCommand(ids.NewID, spec, st, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindReadPrecharge)))
	})

	It("should create close page command with WritePrecharge kind", func() {
		spec.PagePolicy = PagePolicyClose

		ref := subTransRef{TxID: 1, SubIndex: 0}
		cmd := createClosePageCommand(ids.NewID, spec, st, ref)

		Expect(cmd).NotTo(BeNil())
		Expect(cmd.Kind).To(Equal(int(cmdKindWritePrecharge)))
	})

	It("should keep bank open after Read command in open-page mode", func() {
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
		Expect(bankStateKind(bs.State)).To(Equal(bankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(5)))
	})

	It("should keep bank open after Write command in open-page mode", func() {
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
		Expect(bankStateKind(bs.State)).To(Equal(bankStateOpen))
		Expect(bs.OpenRow).To(Equal(uint64(7)))
	})

	It("should close bank after ReadPrecharge command", func() {
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
		Expect(bankStateKind(bs.State)).To(Equal(bankStateClosed))
	})

	It("should close bank after WritePrecharge command", func() {
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
		Expect(bankStateKind(bs.State)).To(Equal(bankStateClosed))
	})

	It("should select command creation based on page policy in tickSubTransQueue", func() {
		// Test with open-page policy
		spec.PagePolicy = PagePolicyOpen
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindRead (not ReadPrecharge)
		Expect(st.CommandQueues.Entries).To(HaveLen(1))
		Expect(st.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindRead)))
	})

	It("should use close-page commands when PagePolicyClose in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 0, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		Expect(progress).To(BeTrue())

		// The command in the queue should be CmdKindReadPrecharge
		Expect(st.CommandQueues.Entries).To(HaveLen(1))
		Expect(st.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindReadPrecharge)))
	})

	It("should use Write for write transaction with open-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyOpen
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		Expect(progress).To(BeTrue())

		Expect(st.CommandQueues.Entries).To(HaveLen(1))
		Expect(st.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindWrite)))
	})

	It("should use WritePrecharge for write transaction with close-page in tickSubTransQueue", func() {
		spec.PagePolicy = PagePolicyClose
		st.SubTransQueue.Entries = []subTransRef{
			{TxID: 1, SubIndex: 0},
		}

		progress := tickSubTransQueue(ids.NewID, spec, st)
		Expect(progress).To(BeTrue())

		Expect(st.CommandQueues.Entries).To(HaveLen(1))
		Expect(st.CommandQueues.Entries[0].Command.Kind).To(
			Equal(int(cmdKindWritePrecharge)))
	})
})

var _ = Describe("FR-FCFS Scheduling", func() {
	var (
		spec *Spec
		st   *state
	)

	BeforeEach(func() {
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
	})

	It("should prioritize row-buffer hits over misses", func() {
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
		Expect(result).NotTo(BeNil())
		// Should pick the hit (row 5) even though it was added second
		Expect(result.Location.Row).To(Equal(uint64(5)))
	})

	It("should use FCFS when no row-buffer hits", func() {
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
		Expect(result).NotTo(BeNil())
		// For closed banks, getRequiredCommandKind returns Activate.
		// The ready command should be an Activate for the older command.
		Expect(result.Kind).To(Equal(int(cmdKindActivate)))
		Expect(result.Location.Row).To(Equal(uint64(10)))
	})

	It("should handle empty queue", func() {
		st.CommandQueues.Entries = []queueEntry{}

		result := getCommandToIssue(spec, st)
		Expect(result).To(BeNil())
	})

	It("should return nil when all commands have timing constraints", func() {
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
		Expect(result).To(BeNil())
	})
})

var _ = Describe("Read/Write Queue Separation", func() {
	var (
		ids  timing.Simulation
		spec *Spec
		st   *state
	)

	BeforeEach(func() {
		ids = modeling.NewStandaloneSimulation(timing.NewSerialEngine())
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
	})

	It("should count write commands", func() {
		st.CommandQueues.Entries = []queueEntry{
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWritePrecharge)}, IsWrite: true},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindRead)}, IsWrite: false},
			{QueueIndex: 0, Command: commandState{Kind: int(cmdKindWrite)}, IsWrite: true},
		}

		count := countWriteCommands(st)
		Expect(count).To(Equal(3))
	})

	It("should count zero writes in empty queue", func() {
		count := countWriteCommands(st)
		Expect(count).To(Equal(0))
	})

	It("should enter drain mode at high watermark", func() {
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

		Expect(st.CommandQueues.WriteDrainMode).To(BeFalse())

		// getCommandToIssue should trigger drain mode
		result := getCommandToIssue(spec, st)
		Expect(result).NotTo(BeNil())
		Expect(st.CommandQueues.WriteDrainMode).To(BeTrue())
	})

	It("should exit drain mode at low watermark", func() {
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
		Expect(result).NotTo(BeNil())
		Expect(st.CommandQueues.WriteDrainMode).To(BeFalse())
	})

	It("should respect separate read/write queue capacities", func() {
		// Fill write queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       ids.NewID(),
				Kind:     int(cmdKindWritePrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(st, cmd, spec)).To(BeTrue())
			acceptCommand(st, cmd)
		}

		// One more write should be rejected
		extraWrite := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, extraWrite, spec)).To(BeFalse())

		// But a read should still be accepted
		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, readCmd, spec)).To(BeTrue())
	})

	It("should respect separate read queue capacity", func() {
		// Fill read queue to capacity (4)
		for i := range 4 {
			cmd := &commandState{
				ID:       ids.NewID(),
				Kind:     int(cmdKindReadPrecharge),
				Location: location{Rank: 0, BankGroup: 0, Bank: uint64(i)},
			}
			Expect(canAcceptCommand(st, cmd, spec)).To(BeTrue())
			acceptCommand(st, cmd)
		}

		// One more read should be rejected
		extraRead := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, extraRead, spec)).To(BeFalse())

		// But a write should still be accepted
		writeCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, writeCmd, spec)).To(BeTrue())
	})

	It("should fall back to unified capacity when sizes are 0", func() {
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
			Expect(canAcceptCommand(st, cmd, spec)).To(BeTrue())
			acceptCommand(st, cmd)
		}

		// Both reads and writes should be rejected
		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, readCmd, spec)).To(BeFalse())

		writeCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0},
		}
		Expect(canAcceptCommand(st, writeCmd, spec)).To(BeFalse())
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
			ID:       ids.NewID(),
			Kind:     int(cmdKindWritePrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(st, writeCmd)
		Expect(st.CommandQueues.Entries[0].IsWrite).To(BeTrue())

		readCmd := &commandState{
			ID:       ids.NewID(),
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0},
		}
		acceptCommand(st, readCmd)
		Expect(st.CommandQueues.Entries[1].IsWrite).To(BeFalse())
	})
})

var _ = Describe("Builder Configuration", func() {
	It("should set page policy via builder", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		spec := Definition.DefaultSpec
		spec.PagePolicy = PagePolicyOpen
		ctrl := buildDRAM(sim, spec, "OpenPageCtrl", 16)

		builtSpec := ctrl.Spec
		Expect(builtSpec.PagePolicy).To(Equal(PagePolicyOpen))
	})

	It("should set R/W queue sizes via builder", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		spec := Definition.DefaultSpec
		spec.ReadQueueSize = 8
		spec.WriteQueueSize = 8
		spec.WriteHighWatermark = 6
		spec.WriteLowWatermark = 2
		ctrl := buildDRAM(sim, spec, "RWQueueCtrl", 16)

		builtSpec := ctrl.Spec
		Expect(builtSpec.ReadQueueSize).To(Equal(8))
		Expect(builtSpec.WriteQueueSize).To(Equal(8))
		Expect(builtSpec.WriteHighWatermark).To(Equal(6))
		Expect(builtSpec.WriteLowWatermark).To(Equal(2))
	})

	It("should build with spec and preserve page policy", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		specWithOpenPage := DDR4Spec
		specWithOpenPage.PagePolicy = PagePolicyOpen
		ctrl := buildDRAM(sim, specWithOpenPage, "DDR4OpenPage", 16)

		builtSpec := ctrl.Spec
		Expect(builtSpec.PagePolicy).To(Equal(PagePolicyOpen))
		Expect(builtSpec.BurstLength).To(Equal(8))
	})

	It("should build with spec and preserve R/W queue config", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		specWithRW := DDR4Spec
		specWithRW.ReadQueueSize = 16
		specWithRW.WriteQueueSize = 16
		specWithRW.WriteHighWatermark = 12
		specWithRW.WriteLowWatermark = 4
		ctrl := buildDRAM(sim, specWithRW, "DDR4RWQueue", 16)

		builtSpec := ctrl.Spec
		Expect(builtSpec.ReadQueueSize).To(Equal(16))
		Expect(builtSpec.WriteQueueSize).To(Equal(16))
		Expect(builtSpec.WriteHighWatermark).To(Equal(12))
		Expect(builtSpec.WriteLowWatermark).To(Equal(4))
	})

	It("should require a storage", func() {
		sim := modeling.NewStandaloneSimulation(timing.NewSerialEngine())

		Expect(func() {
			Definition.Builder().
				WithSimulation(sim).
				WithPorts(defaultPorts("NoStorage", 16)).
				Build("NoStorage")
		}).To(PanicWith("dram: Resources.Storage is required"))
	})
})
