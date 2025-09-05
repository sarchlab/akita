package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMU", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		topPort       *MockPort
		migrationPort *MockPort
		pageTable     *MockPageTable
		mmu           *Comp
		mmuMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		migrationPort = NewMockPort(mockCtrl)
		migrationPort.EXPECT().AsRemote().
			Return(sim.RemotePort("MigrationPort")).
			AnyTimes()

		builder := MakeBuilder().WithEngine(engine)
		mmu = builder.Build("MMU")
		mmu.topPort = topPort
		mmu.migrationPort = migrationPort
		mmu.pageTable = pageTable
		mmu.MigrationServiceProvider =
			sim.RemotePort("MigrationServiceProvider")

		mmuMiddleware = mmu.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("parse top", func() {
		It("should process translation request", func() {
			translationReq := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x100000100).
				WithDeviceID(0).
				Build()
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
			req := vm.TranslationReqBuilder{}.
				WithDst(topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1020).
				WithDeviceID(0).
				Build()
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
			req := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp *vm.TranslationRsp) {
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
			req := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
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
			req     *vm.TranslationReq
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
			req = vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
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

		It("should place the page in the migration queue "+
			"if the page is being migrated", func() {
			req.PID = 2
			page.PID = 2
			page.IsMigrating = true
			pageTable.EXPECT().
				Find(vm.PID(2), uint64(0x1000)).
				Return(page, true)
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().Update(page)

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
			Expect(mmu.migrationQueue).To(HaveLen(1))
		})

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
				Return(sim.NewSendError())

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
			req           *vm.TranslationReq
			migrating     transaction
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
			req = vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			migrating = transaction{req: req, cycleLeft: 0}
			mmu.currentOnDemandMigration = migrating
			migrationDone = vm.NewPageMigrationRspFromDriver("", "", req)
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
				Do(func(rsp *vm.TranslationRsp) {
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

	Context("auto page allocation", func() {
		BeforeEach(func() {
			// Enable auto page allocation for these tests
			mmu.autoPageAllocation = true
			mmu.log2PageSize = 12 // 4KB pages
		})

		It("should create a new page when page not found and auto allocation is enabled", func() {
			req := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(2).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			// Page not found initially
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(vm.Page{}, false)
			
			// Page should be inserted
			pageTable.EXPECT().
				Insert(gomock.Any()).
				Do(func(page vm.Page) {
					Expect(page.PID).To(Equal(vm.PID(1)))
					Expect(page.VAddr).To(Equal(uint64(0x1000)))
					Expect(page.PageSize).To(Equal(uint64(4096)))
					Expect(page.Valid).To(BeTrue())
					Expect(page.DeviceID).To(Equal(uint64(2)))
					Expect(page.Unified).To(BeTrue())
					Expect(page.IsMigrating).To(BeFalse())
					Expect(page.IsPinned).To(BeFalse())
				})

			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp *vm.TranslationRsp) {
					Expect(rsp.Page.PID).To(Equal(vm.PID(1)))
					Expect(rsp.Page.VAddr).To(Equal(uint64(0x1000)))
					Expect(rsp.Page.PageSize).To(Equal(uint64(4096)))
				})

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
		})

		It("should still panic when auto allocation is disabled and page not found", func() {
			// Disable auto page allocation
			mmu.autoPageAllocation = false
			
			req := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(2).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			// Page not found
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(vm.Page{}, false)

			Expect(func() {
				mmuMiddleware.walkPageTable()
			}).To(Panic())
		})

		It("should align virtual addresses to page boundaries", func() {
			req := vm.TranslationReqBuilder{}.
				WithDst(mmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x1234). // Unaligned address
				WithDeviceID(2).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			// Page not found initially
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1234)).
				Return(vm.Page{}, false)
			
			// Page should be inserted with aligned address
			pageTable.EXPECT().
				Insert(gomock.Any()).
				Do(func(page vm.Page) {
					Expect(page.VAddr).To(Equal(uint64(0x1000))) // Should be aligned to 4KB boundary
				})

			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any())

			madeProgress := mmuMiddleware.walkPageTable()

			Expect(madeProgress).To(BeTrue())
		})
	})
})

var _ = Describe("MMU Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		mmu        *Comp
		agent      *MockPort
		connection sim.Connection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()

		builder := MakeBuilder().WithEngine(engine)
		mmu = builder.Build("MMU")

		agent = NewMockPort(mockCtrl)
		agent.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		agent.EXPECT().AsRemote().Return(sim.RemotePort("Agent")).AnyTimes()

		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
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

		req := vm.TranslationReqBuilder{}.
			WithSrc(agent.AsRemote()).
			WithDst(mmu.topPort.AsRemote()).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(0).
			Build()
		mmu.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
				Expect(rsp.RespondTo).To(Equal(req.ID))
			})

		engine.Run()
	})

	It("should work with auto page allocation enabled", func() {
		// Create MMU with auto page allocation enabled
		builder := MakeBuilder().
			WithEngine(engine).
			WithAutoPageAllocation(true)
		mmu = builder.Build("MMU")

		// Reconnect with new MMU
		agent.EXPECT().SetConnection(connection)
		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		connection.PlugIn(agent)
		connection.PlugIn(mmu.topPort)

		// Request translation for a page that doesn't exist
		req := vm.TranslationReqBuilder{}.
			WithSrc(agent.AsRemote()).
			WithDst(mmu.topPort.AsRemote()).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(2).
			Build()
		mmu.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				// Should receive a response with a newly created page
				Expect(rsp.Page.PID).To(Equal(vm.PID(1)))
				Expect(rsp.Page.VAddr).To(Equal(uint64(0x1000)))
				Expect(rsp.Page.Valid).To(BeTrue())
				Expect(rsp.Page.DeviceID).To(Equal(uint64(2)))
				Expect(rsp.RespondTo).To(Equal(req.ID))
			})

		engine.Run()
	})
})
