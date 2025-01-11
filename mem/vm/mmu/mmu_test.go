package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMU", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		simulation    *MockSimulation
		topPort       *MockPort
		migrationPort *MockPort
		pageTable     *MockPageTable
		mmu           *Comp
		mmuMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		simulation = NewMockSimulation(mockCtrl)
		simulation.EXPECT().GetEngine().Return(engine).AnyTimes()
		simulation.EXPECT().
			RegisterStateHolder(gomock.Any()).
			Return().
			AnyTimes()

		pageTable = NewMockPageTable(mockCtrl)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().
			Return(modeling.RemotePort("TopPort")).
			AnyTimes()

		migrationPort = NewMockPort(mockCtrl)
		migrationPort.EXPECT().AsRemote().
			Return(modeling.RemotePort("MigrationPort")).
			AnyTimes()

		builder := MakeBuilder().WithSimulation(simulation)
		mmu = builder.Build("MMU")
		mmu.topPort = topPort
		mmu.migrationPort = migrationPort
		mmu.pageTable = pageTable
		mmu.MigrationServiceProvider =
			modeling.RemotePort("MigrationServiceProvider")

		mmuMiddleware = mmu.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("parse top", func() {
		It("should process translation request", func() {
			translationReq := vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x100000100,
				DeviceID: 0,
			}
			topPort.EXPECT().
				RetrieveIncoming().
				Return(translationReq)

			mmuMiddleware.parseFromTop()

			Expect(mmu.walkingTranslations).To(HaveLen(1))

		})

		It("should stall parse from top "+
			"if MMU is servicing max requests",
			func() {
				mmu.walkingTranslations = make([]transaction, 16)

				madeProgress := mmuMiddleware.parseFromTop()

				Expect(madeProgress).To(BeFalse())
			})
	})

	Context("walk page table", func() {
		It("should reduce translation cycles", func() {
			req := vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x1020,
				DeviceID: 0,
			}
			walking := transaction{req: req, cycleLeft: 10}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(mmu.walkingTranslations[0].cycleLeft).To(Equal(9))
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
			req := vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x1000,
				DeviceID: 0,
			}
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp vm.TranslationRsp) {
					Expect(rsp.Page).To(Equal(page))
				})

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
		})

		It("should stall if cannot send to top", func() {
			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}
			req := vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x1000,
				DeviceID: 0,
			}
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations =
				append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topPort.EXPECT().CanSend().Return(false)

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("migration", func() {
		var (
			page    vm.Page
			req     vm.TranslationReq
			walking transaction
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
			req = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x1000,
				DeviceID: 0,
			}
			walking = transaction{
				req:       req,
				page:      page,
				cycleLeft: 0,
			}
		})

		It("should be placed in the migration queue", func() {
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
			Expect(mmu.migrationQueue).To(HaveLen(1))
		})

		// It("should place the page in the migration queue "+
		// 	"if the page is being migrated", func() {
		// 	req.PID = 2
		// 	page.PID = 2
		// 	page.IsMigrating = true
		// 	pageTable.EXPECT().
		// 		Find(vm.PID(2), uint64(0x1000)).
		// 		Return(page, true)
		// 	mmu.walkingTranslations =
		// append(mmu.walkingTranslations, walking)

		// 	pageTable.EXPECT().Update(page)

		// 	madeProgress := mmuMiddleware.walkPageTable()

		// 	Expect(madeProgress).To(BeTrue())
		// 	Expect(mmu.walkingTranslations).To(HaveLen(0))
		// 	Expect(mmu.migrationQueue).To(HaveLen(1))
		// })

		It("should not send to driver if migration queue is empty", func() {
			madeProgress := mmuMiddleware.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait if mmu is waiting for a migration to finish", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)
			mmu.isDoingMigration = true

			madeProgress := mmuMiddleware.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.migrationQueue).To(ContainElement(walking))
		})

		It("should stall if send failed", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(modeling.NewSendError())

			madeProgress := mmuMiddleware.sendMigrationToDriver()

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.migrationQueue).To(ContainElement(walking))
		})

		It("should send migration request", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(nil)
			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmuMiddleware.sendMigrationToDriver()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.migrationQueue).NotTo(ContainElement(walking))
			Expect(mmu.isDoingMigration).To(BeTrue())
		})

		It("should reply to the GPU if the page is already on the "+
			"destination GPU", func() {
			walking.req.DeviceID = 2
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any())

			madeProgress := mmuMiddleware.sendMigrationToDriver()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.migrationQueue).NotTo(ContainElement(walking))
			Expect(mmu.isDoingMigration).To(BeFalse())
		})
	})

	Context("when received migrated page information", func() {
		var (
			page          vm.Page
			req           vm.TranslationReq
			migrating     transaction
			migrationDone vm.PageMigrationRspFromDriver
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
			req = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.topPort.AsRemote(),
					Dst: mmu.MigrationServiceProvider,
				},
				PID:      1,
				VAddr:    0x1000,
				DeviceID: 0,
			}
			migrating = transaction{req: req, cycleLeft: 0}
			mmu.currentOnDemandMigration = migrating
			migrationDone = vm.PageMigrationRspFromDriver{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: mmu.MigrationServiceProvider,
					Dst: mmu.topPort.AsRemote(),
				},
			}
		})

		It("should do nothing if no respond", func() {
			migrationPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := mmuMiddleware.processMigrationReturn()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to top failed", func() {
			migrationPort.EXPECT().PeekIncoming().Return(migrationDone)
			topPort.EXPECT().CanSend().Return(false)

			madeProgress := mmuMiddleware.processMigrationReturn()

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.isDoingMigration).To(BeFalse())
		})

		It("should send rsp to top", func() {
			migrationPort.EXPECT().PeekIncoming().Return(migrationDone)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(rsp vm.TranslationRsp) {
					Expect(rsp.Page).To(Equal(page))
				})
			migrationPort.EXPECT().RetrieveIncoming()

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)

			updatedPage.IsPinned = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmuMiddleware.processMigrationReturn()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.isDoingMigration).To(BeFalse())
		})

	})
})

var _ = Describe("MMU Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     timing.Engine
		simulation *MockSimulation
		mmu        *Comp
		agent      *MockPort
		connection modeling.Connection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()

		simulation = NewMockSimulation(mockCtrl)
		simulation.EXPECT().GetEngine().Return(engine).AnyTimes()
		simulation.EXPECT().
			RegisterStateHolder(gomock.Any()).
			Return().
			AnyTimes()

		builder := MakeBuilder().WithSimulation(simulation)
		mmu = builder.Build("MMU")

		agent = NewMockPort(mockCtrl)
		agent.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		agent.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("Agent")).
			AnyTimes()

		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * timing.GHz).
			Build("Conn")

		agent.EXPECT().SetConnection(connection)
		connection.PlugIn(agent)
		connection.PlugIn(mmu.topPort)
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
		mmu.pageTable.Insert(page)

		req := vm.TranslationReq{
			MsgMeta: modeling.MsgMeta{
				ID:  id.Generate(),
				Src: agent.AsRemote(),
				Dst: mmu.topPort.AsRemote(),
			},
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 0,
		}
		mmu.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
				Expect(rsp.RespondTo).To(Equal(req.ID))
			})

		engine.Run()
	})
})
