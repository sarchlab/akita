package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"
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
							State: int(BankStateClosed),
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
							State: int(BankStateOpen),
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
