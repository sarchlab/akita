package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("MMU remote faults", func() {
	var (
		pageTable vm.PageTable
		mmuComp   *Comp
		topPort   messaging.Port
		faultPort messaging.Port
		mw        *translationMW
	)

	BeforeEach(func() {
		reg := modeling.NewStandaloneRegistrar(timing.NewSerialEngine())
		pageTable = vm.NewPageTable(12)

		spec := DefaultSpec()
		spec.RemoteFaults = true
		spec.FaultHandler = messaging.RemotePort("Driver.PageMigration")

		mmuComp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("MMU")

		topPort = assignPort(reg, mmuComp, "Top", 16)
		faultPort = assignPort(reg, mmuComp, "Fault", 16)
		assignPort(reg, mmuComp, "Control", 4)

		for _, p := range []messaging.Port{
			topPort, faultPort, mmuComp.GetPortByName("Control"),
		} {
			(&noopConn{}).PlugIn(p)
		}

		mw = mmuComp.Middlewares()[1].(*translationMW)
	})

	walk := func(page vm.Page, canFault bool) {
		pageTable.Insert(page)

		mmuComp.State.WalkingTranslations = []transactionState{{
			ReqID:    timing.GetIDGenerator().Generate(),
			ReqSrc:   messaging.RemotePort("L2TLB.Bottom"),
			PID:      uint32(page.PID),
			VAddr:    page.VAddr,
			DeviceID: 1,
			CanFault: canFault,
		}}

		Expect(mw.walkPageTable()).To(BeTrue())
	}

	remotePage := vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000,
		DeviceID: 2, Valid: true}

	It("answers a walk of a remote page with a fault and reports it", func() {
		walk(remotePage, true)

		rsp := topPort.RetrieveOutgoing().(vmprotocol.TranslationRsp)
		Expect(rsp.Fault).To(BeTrue())
		Expect(rsp.Page.VAddr).To(Equal(uint64(0x1000)))

		fault := faultPort.RetrieveOutgoing().(vmprotocol.PageFault)
		Expect(fault.Dst).To(Equal(messaging.RemotePort("Driver.PageMigration")))
		Expect(fault.VAddr).To(Equal(uint64(0x1000)))
		Expect(fault.DeviceID).To(Equal(uint64(1)))
		Expect(fault.Page.DeviceID).To(Equal(uint64(2)))

		Expect(mmuComp.State.WalkingTranslations).To(BeEmpty())
		Expect(mmuComp.State.NumFaults).To(Equal(uint64(1)))
	})

	It("does not fault when the request may not fault", func() {
		walk(remotePage, false)

		rsp := topPort.RetrieveOutgoing().(vmprotocol.TranslationRsp)
		Expect(rsp.Fault).To(BeFalse())
		Expect(faultPort.RetrieveOutgoing()).To(BeNil())
	})

	It("does not fault on a pinned page", func() {
		pinned := remotePage
		pinned.IsPinned = true
		walk(pinned, true)

		rsp := topPort.RetrieveOutgoing().(vmprotocol.TranslationRsp)
		Expect(rsp.Fault).To(BeFalse())
		Expect(faultPort.RetrieveOutgoing()).To(BeNil())
	})

	It("does not fault on a local page", func() {
		local := remotePage
		local.DeviceID = 1
		walk(local, true)

		rsp := topPort.RetrieveOutgoing().(vmprotocol.TranslationRsp)
		Expect(rsp.Fault).To(BeFalse())
	})
})
