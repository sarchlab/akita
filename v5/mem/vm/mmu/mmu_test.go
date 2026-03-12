package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	simengine "github.com/sarchlab/akita/v5/sim/engine"
	"github.com/sarchlab/akita/v5/sim/directconnection"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMU", func() {

	var (
		mockCtrl         *gomock.Controller
		engine           *MockEngine
		topPort          *MockPort
		migrationPort    *MockPort
		pageTable        *MockPageTable
		mmuComp          *modeling.Component[Spec, State]
		translationMWRef *translationMW
		migrationMWRef   *migrationMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		migrationPort = NewMockPort(mockCtrl)
		migrationPort.EXPECT().AsRemote().
			Return(sim.RemotePort("MigrationPort")).
			AnyTimes()
		migrationPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		builder := MakeBuilder().
			WithEngine(engine).
			WithTopPort(topPort).
			WithMigrationPort(migrationPort).
			WithMigrationServiceProvider(sim.RemotePort("MigrationServiceProvider"))
		mmuComp = builder.Build("MMU")

		translationMWRef = mmuComp.Middlewares()[0].(*translationMW)
		translationMWRef.pageTable = pageTable

		migrationMWRef = mmuComp.Middlewares()[1].(*migrationMW)
		migrationMWRef.pageTable = pageTable
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("parse top", func() {
		It("should process translation request", func() {
			translationReq := &vm.TranslationReq{}
			translationReq.ID = sim.GetIDGenerator().Generate()
			translationReq.Dst = sim.RemotePort("TopPort")
			translationReq.PID = 1
			translationReq.VAddr = 0x100000100
			translationReq.DeviceID = 0
			translationReq.TrafficClass = "vm.TranslationReq"
			topPort.EXPECT().
				RetrieveIncoming().
				Return(translationReq)

			translationMWRef.parseFromTop()

			next := mmuComp.GetNextState()
			Expect(next.WalkingTranslations).To(HaveLen(1))
		})

		It("should stall parse from top "+
			"if MMU is servicing max requests",
			func() {
				mmuComp.SetState(State{
					WalkingTranslations: make([]transactionState, 16),
				})

				madeProgress := translationMWRef.parseFromTop()

				Expect(madeProgress).To(BeFalse())
			})
	})

	Context("walk page table", func() {
		It("should reduce translation cycles", func() {
			mmuComp.SetState(State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.GetIDGenerator().Generate(),
						ReqDst:    sim.RemotePort("TopPort"),
						PID:       1,
						VAddr:     0x1020,
						DeviceID:  0,
						CycleLeft: 10,
					},
				},
			})

			madeProgress := translationMWRef.walkPageTable()

			next := mmuComp.GetNextState()
			Expect(next.WalkingTranslations[0].CycleLeft).To(Equal(9))
			Expect(madeProgress).To(BeTrue())
		})

		It("should send rsp to top if hit", func() {
			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}

			mmuComp.SetState(State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.GetIDGenerator().Generate(),
						ReqSrc:    sim.RemotePort("Agent"),
						ReqDst:    sim.RemotePort("TopPort"),
						PID:       1,
						VAddr:     0x1000,
						DeviceID:  0,
						CycleLeft: 0,
					},
				},
			})

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					rsp := msg.(*vm.TranslationRsp)
					Expect(rsp.Page).To(Equal(page))
				})

			madeProgress := translationMWRef.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.WalkingTranslations).To(HaveLen(0))
		})

		It("should stall if cannot send to top", func() {
			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}

			mmuComp.SetState(State{
				WalkingTranslations: []transactionState{
					{
						ReqID:     sim.GetIDGenerator().Generate(),
						ReqSrc:    sim.RemotePort("Agent"),
						ReqDst:    sim.RemotePort("TopPort"),
						PID:       1,
						VAddr:     0x1000,
						DeviceID:  0,
						CycleLeft: 0,
					},
				},
			})

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topPort.EXPECT().CanSend().Return(false)

			madeProgress := translationMWRef.walkPageTable()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("migration", func() {
		var (
			page    vm.Page
			walking transactionState
		)

		BeforeEach(func() {
			page = vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
				DeviceID: 2,
				Unified:  true,
			}
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true).
				AnyTimes()

			walking = transactionState{
				ReqID:     sim.GetIDGenerator().Generate(),
				ReqSrc:    sim.RemotePort("Agent"),
				ReqDst:    sim.RemotePort("TopPort"),
				PID:       1,
				VAddr:     0x1000,
				DeviceID:  0,
				Page:      page,
				CycleLeft: 0,
			}
		})

		It("should be placed in the migration queue", func() {
			mmuComp.SetState(State{
				WalkingTranslations: []transactionState{walking},
			})

			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := translationMWRef.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.WalkingTranslations).To(HaveLen(0))
			Expect(next.MigrationQueue).To(HaveLen(1))
		})

		It("should place the page in the migration queue "+
			"if the page is being migrated", func() {
			walking.PID = 2
			page.PID = 2
			page.IsMigrating = true
			pageTable.EXPECT().
				Find(vm.PID(2), uint64(0x1000)).
				Return(page, true)

			mmuComp.SetState(State{
				WalkingTranslations: []transactionState{walking},
			})

			pageTable.EXPECT().Update(page)

			madeProgress := translationMWRef.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.WalkingTranslations).To(HaveLen(0))
			Expect(next.MigrationQueue).To(HaveLen(1))
		})

		It("should not send to driver if migration queue is empty", func() {
			madeProgress := migrationMWRef.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait if mmu is waiting for a migration to finish", func() {
			mmuComp.SetState(State{
				MigrationQueue:   []transactionState{walking},
				IsDoingMigration: true,
			})

			madeProgress := migrationMWRef.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
			next := mmuComp.GetNextState()
			Expect(next.MigrationQueue).To(HaveLen(1))
		})

		It("should stall if send failed", func() {
			mmuComp.SetState(State{
				MigrationQueue: []transactionState{walking},
			})

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(sim.NewSendError())

			madeProgress := migrationMWRef.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
			next := mmuComp.GetNextState()
			Expect(next.MigrationQueue).To(HaveLen(1))
		})

		It("should send migration request", func() {
			mmuComp.SetState(State{
				MigrationQueue: []transactionState{walking},
			})

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(nil)
			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := migrationMWRef.sendMigrationToDriver()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.MigrationQueue).To(HaveLen(0))
			Expect(next.IsDoingMigration).To(BeTrue())
		})

		It("should reply to the GPU if the page is already on the "+
			"destination GPU", func() {
			walking.DeviceID = 2
			mmuComp.SetState(State{
				MigrationQueue: []transactionState{walking},
			})

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any())

			madeProgress := migrationMWRef.sendMigrationToDriver()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.MigrationQueue).To(HaveLen(0))
			Expect(next.IsDoingMigration).To(BeFalse())
		})
	})

	Context("when received migrated page information", func() {
		var (
			page          vm.Page
			migrating     transactionState
			migrationDone *vm.PageMigrationRspFromDriver
		)

		BeforeEach(func() {
			page = vm.Page{
				PID:         1,
				VAddr:       0x1000,
				PAddr:       0x0,
				PageSize:    4096,
				Valid:       true,
				DeviceID:    1,
				Unified:     true,
				IsMigrating: true,
			}
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true).
				AnyTimes()

			reqID := sim.GetIDGenerator().Generate()
			migrating = transactionState{
				ReqID:     reqID,
				ReqSrc:    sim.RemotePort("Agent"),
				ReqDst:    sim.RemotePort("TopPort"),
				PID:       1,
				VAddr:     0x1000,
				DeviceID:  0,
				CycleLeft: 0,
			}
			mmuComp.SetState(State{
				CurrentOnDemandMigration: migrating,
			})

			migrationDone = vm.NewPageMigrationRspFromDriver(
				"", "", reqID)
		})

		It("should do nothing if no respond", func() {
			migrationPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := migrationMWRef.processMigrationReturn()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to top failed", func() {
			migrationPort.EXPECT().PeekIncoming().Return(migrationDone)
			topPort.EXPECT().CanSend().Return(false)

			madeProgress := migrationMWRef.processMigrationReturn()

			Expect(madeProgress).To(BeFalse())
			next := mmuComp.GetNextState()
			Expect(next.IsDoingMigration).To(BeFalse())
		})

		It("should send rsp to top", func() {
			migrationPort.EXPECT().PeekIncoming().Return(migrationDone)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					rsp := msg.(*vm.TranslationRsp)
					Expect(rsp.Page).To(Equal(page))
				})
			migrationPort.EXPECT().RetrieveIncoming()

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)

			updatedPage.IsPinned = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := migrationMWRef.processMigrationReturn()

			Expect(madeProgress).To(BeTrue())
			next := mmuComp.GetNextState()
			Expect(next.IsDoingMigration).To(BeFalse())
		})

	})
})

var _ = Describe("MMU Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		mmuComp    *modeling.Component[Spec, State]
		agent      *MockPort
		connection sim.Connection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = simengine.NewSerialEngine()

		builder := MakeBuilder().
			WithEngine(engine).
			WithTopPort(sim.NewPort(nil, 4096, 4096, "MMU.ToTop")).
			WithMigrationPort(sim.NewPort(nil, 1, 1, "MMU.MigrationPort"))
		mmuComp = builder.Build("MMU")

		agent = NewMockPort(mockCtrl)
		agent.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		agent.EXPECT().AsRemote().Return(sim.RemotePort("Agent")).AnyTimes()

		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")

		topPort := mmuComp.GetPortByName("Top")

		agent.EXPECT().SetConnection(connection)
		connection.PlugIn(agent)
		connection.PlugIn(topPort)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should lookup", func() {
		page := vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 1,
		}
		PageTable(mmuComp).Insert(page)

		topPort := mmuComp.GetPortByName("Top")

		req := &vm.TranslationReq{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = agent.AsRemote()
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 0
		req.TrafficClass = "vm.TranslationReq"
		topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				rsp := msg.(*vm.TranslationRsp)
				Expect(rsp.Page).To(Equal(page))
				Expect(rsp.RspTo).To(Equal(req.ID))
			})

		engine.Run()
	})
})
