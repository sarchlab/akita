package addresstranslator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// testMigrationHook holds every access while blocked and, once unblocked,
// remaps the accesses to remap (when set).
type testMigrationHook struct {
	blocked   bool
	remap     *vm.Page
	wakers    []Waker
	issued    []uint64
	completed []uint64
}

func (h *testMigrationHook) Resolve(
	waker Waker,
	_ vm.PID,
	_ uint64,
	page vm.Page,
) (vm.Page, bool) {
	if h.blocked {
		h.wakers = append(h.wakers, waker)
		return page, false
	}

	if h.remap != nil {
		return *h.remap, true
	}

	return page, true
}

func (h *testMigrationHook) Issued(_ vm.PID, _, pAddr uint64) {
	h.issued = append(h.issued, pAddr)
}

func (h *testMigrationHook) Completed(pAddr uint64) {
	h.completed = append(h.completed, pAddr)
}

var _ = Describe("Address Translator with a migration hook", func() {
	var (
		t               *Comp
		hook            *testMigrationHook
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
	)

	BeforeEach(func() {
		hook = &testMigrationHook{blocked: true}

		reg := modeling.NewStandaloneRegistrar(timing.NewSerialEngine())
		t = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{
				MemProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("MemPort"),
				},
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("TranslationProvider"),
				},
				MigrationHook: hook,
			}).
			Build("AddressTranslator")
		assignPorts(reg, t, topBufSize)

		topPort = t.GetPortByName("Top")
		bottomPort = t.GetPortByName("Bottom")
		translationPort = t.GetPortByName("Translation")
		ctrlPort = t.GetPortByName("Control")

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			(&noopConn{}).PlugIn(p)
		}
	})

	// translate delivers a read of vAddr and answers its translation with
	// page.
	translate := func(vAddr uint64, page vm.Page) {
		req := memprotocol.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = vAddr
		req.AccessByteSize = 4
		req.PID = 1
		req.TrafficClass = "memprotocol.ReadReq"
		topPort.Deliver(req)

		t.Tick()

		transReq := translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq)
		rsp := vmprotocol.TranslationRsp{Page: page}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.RspTo = transReq.ID
		rsp.TrafficClass = "vmprotocol.TranslationRsp"
		translationPort.Deliver(rsp)

		t.Tick()
	}

	oldPage := vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x1000, Valid: true}
	newPage := vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x2000, Valid: true}

	It("holds an access and sends it with the remapped page", func() {
		translate(0x10040, oldPage)

		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
		Expect(t.State.HeldReqs).To(HaveLen(1))
		Expect(t.State.NumHeldReqs).To(Equal(uint64(1)))
		Expect(t.State.Transactions).To(BeEmpty())
		Expect(hook.issued).To(BeEmpty())
		Expect(hook.wakers).To(ContainElement(Waker(t)))

		// Still blocked: ticking again does not release it.
		t.Tick()
		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())

		hook.blocked = false
		hook.remap = &newPage
		t.Tick()

		sent := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)
		Expect(sent.Address).To(Equal(uint64(0x2040)))
		Expect(t.State.HeldReqs).To(BeEmpty())
		Expect(hook.issued).To(Equal([]uint64{0x2040}))

		dataReady := memprotocol.DataReadyRsp{Data: []byte{7, 7, 7, 7}}
		dataReady.ID = timing.GetIDGenerator().Generate()
		dataReady.RspTo = sent.ID
		dataReady.TrafficClass = "memprotocol.DataReadyRsp"
		bottomPort.Deliver(dataReady)
		t.Tick()

		rsp := topPort.RetrieveOutgoing().(memprotocol.DataReadyRsp)
		Expect(rsp.Data).To(Equal([]byte{7, 7, 7, 7}))
		Expect(hook.completed).To(Equal([]uint64{0x2040}))
	})

	It("reports the issue and completion of accesses it does not hold", func() {
		hook.blocked = false
		translate(0x10040, oldPage)

		sent := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)
		Expect(sent.Address).To(Equal(uint64(0x1040)))
		Expect(t.State.NumHeldReqs).To(BeZero())
		Expect(hook.issued).To(Equal([]uint64{0x1040}))

		dataReady := memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}
		dataReady.ID = timing.GetIDGenerator().Generate()
		dataReady.RspTo = sent.ID
		dataReady.TrafficClass = "memprotocol.DataReadyRsp"
		bottomPort.Deliver(dataReady)
		t.Tick()

		Expect(hook.completed).To(Equal([]uint64{0x1040}))
	})

	It("does not complete a Drain while accesses are held", func() {
		translate(0x10040, oldPage)

		drain := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdDrain}
		drain.ID = timing.GetIDGenerator().Generate()
		drain.Src = messaging.RemotePort("Ctrl")
		drain.Dst = ctrlPort.AsRemote()
		ctrlPort.Deliver(drain)

		for range 4 {
			t.Tick()
		}
		Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())

		hook.blocked = false
		t.Tick()
		Expect(bottomPort.RetrieveOutgoing()).NotTo(BeNil())
		Expect(t.State.HeldReqs).To(BeEmpty())
	})

	It("drops held accesses on Reset", func() {
		translate(0x10040, oldPage)
		Expect(t.State.HeldReqs).To(HaveLen(1))

		reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
		reset.ID = timing.GetIDGenerator().Generate()
		reset.Src = messaging.RemotePort("Ctrl")
		reset.Dst = ctrlPort.AsRemote()
		ctrlPort.Deliver(reset)
		t.Tick()

		rsp := ctrlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
		Expect(rsp.Success).To(BeTrue())
		Expect(t.State.HeldReqs).To(BeEmpty())
	})
})
