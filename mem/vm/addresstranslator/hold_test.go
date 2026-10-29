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

var _ = Describe("Address Translator Hold, Release, and faults", func() {
	var (
		t               *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
	)

	BeforeEach(func() {
		reg := modeling.NewStandaloneRegistrar(timing.NewSerialEngine())
		spec := DefaultSpec()
		spec.CanFault = true
		t = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				MemProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("MemPort"),
				},
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("TranslationProvider"),
				},
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

	page := vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x1000, Valid: true}

	access := func(vAddr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = vAddr
		req.AccessByteSize = 4
		req.PID = 1
		req.TrafficClass = "memprotocol.ReadReq"
		topPort.Deliver(req)

		return req
	}

	answer := func(transReq vmprotocol.TranslationReq, fault bool) {
		rsp := vmprotocol.TranslationRsp{Page: page, Fault: fault}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.RspTo = transReq.ID
		rsp.TrafficClass = "vmprotocol.TranslationRsp"
		translationPort.Deliver(rsp)
	}

	control := func(cmd memcontrolprotocol.Command, vAddr uint64) uint64 {
		req := memcontrolprotocol.Req{
			Command:   cmd,
			Addresses: []uint64{vAddr},
			PID:       1,
		}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Driver")
		req.Dst = ctrlPort.AsRemote()
		ctrlPort.Deliver(req)

		return req.ID
	}

	ack := func() (memcontrolprotocol.Rsp, bool) {
		msg := ctrlPort.RetrieveOutgoing()
		if msg == nil {
			return memcontrolprotocol.Rsp{}, false
		}

		return msg.(memcontrolprotocol.Rsp), true
	}

	It("sets CanFault on its translations", func() {
		access(0x10040)
		t.Tick()

		transReq := translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq)
		Expect(transReq.CanFault).To(BeTrue())
		Expect(transReq.VAddr).To(Equal(uint64(0x10000)))
	})

	It("holds a new access to a held page without translating it", func() {
		holdID := control(memcontrolprotocol.CmdHold, 0x10000)
		t.Tick()

		rsp, ok := ack()
		Expect(ok).To(BeTrue())
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdHold))
		Expect(rsp.RspTo).To(Equal(holdID))

		access(0x10040)
		t.Tick()

		Expect(translationPort.RetrieveOutgoing()).To(BeNil())
		Expect(t.State.HeldReqs).To(HaveLen(1))

		control(memcontrolprotocol.CmdRelease, 0x10000)
		t.Tick()

		rsp, ok = ack()
		Expect(ok).To(BeTrue())
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdRelease))

		t.Tick()

		transReq := translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq)
		Expect(transReq.VAddr).To(Equal(uint64(0x10000)))
		Expect(t.State.HeldReqs).To(BeEmpty())
		Expect(t.State.ReplayReqs).To(BeEmpty())
	})

	It("does not hold the accesses to other pages", func() {
		control(memcontrolprotocol.CmdHold, 0x10000)
		t.Tick()
		ack()

		access(0x20040)
		t.Tick()

		Expect(translationPort.RetrieveOutgoing()).NotTo(BeNil())
	})

	It("acknowledges a Hold only after the translation in flight returns",
		func() {
			access(0x10040)
			t.Tick()
			transReq := translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq)

			control(memcontrolprotocol.CmdHold, 0x10000)
			t.Tick()
			_, ok := ack()
			Expect(ok).To(BeFalse())

			// The translation returns after the Hold: the access is held,
			// not sent with a translation that may be stale.
			answer(transReq, false)
			t.Tick()
			Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
			Expect(t.State.HeldReqs).To(HaveLen(1))

			t.Tick()
			rsp, ok := ack()
			Expect(ok).To(BeTrue())
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdHold))
		})

	It("acknowledges a Hold only after the access in flight completes",
		func() {
			access(0x10040)
			t.Tick()
			answer(translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq),
				false)
			t.Tick()
			read := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)

			control(memcontrolprotocol.CmdHold, 0x10000)
			t.Tick()
			t.Tick()
			_, ok := ack()
			Expect(ok).To(BeFalse())

			dataReady := memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = read.ID
			dataReady.TrafficClass = "memprotocol.DataReadyRsp"
			bottomPort.Deliver(dataReady)
			t.Tick()
			t.Tick()

			rsp, ok := ack()
			Expect(ok).To(BeTrue())
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdHold))
		})

	It("holds a faulted access and the later accesses to its page "+
		"until a Release", func() {
		access(0x10040)
		t.Tick()
		answer(translationPort.RetrieveOutgoing().(vmprotocol.TranslationReq),
			true)
		t.Tick()

		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
		Expect(t.State.NumFaults).To(Equal(uint64(1)))
		Expect(t.State.HeldReqs).To(HaveLen(1))

		access(0x10080)
		t.Tick()
		Expect(translationPort.RetrieveOutgoing()).To(BeNil())
		Expect(t.State.HeldReqs).To(HaveLen(2))

		control(memcontrolprotocol.CmdRelease, 0x10000)
		t.Tick()
		_, ok := ack()
		Expect(ok).To(BeTrue())

		t.Tick()
		Expect(translationPort.RetrieveOutgoing()).NotTo(BeNil())
		Expect(translationPort.RetrieveOutgoing()).NotTo(BeNil())
		Expect(t.State.Transactions).To(HaveLen(2))
	})

	It("does not complete a Drain while accesses are held", func() {
		control(memcontrolprotocol.CmdHold, 0x10000)
		t.Tick()
		ack()

		access(0x10040)
		t.Tick()

		drain := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdDrain}
		drain.ID = timing.GetIDGenerator().Generate()
		drain.Src = messaging.RemotePort("Driver")
		drain.Dst = ctrlPort.AsRemote()
		ctrlPort.Deliver(drain)
		t.Tick()
		t.Tick()

		_, ok := ack()
		Expect(ok).To(BeFalse())
	})

	It("drops the held accesses on Reset", func() {
		control(memcontrolprotocol.CmdHold, 0x10000)
		t.Tick()
		ack()

		access(0x10040)
		t.Tick()

		reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
		reset.ID = timing.GetIDGenerator().Generate()
		reset.Src = messaging.RemotePort("Driver")
		reset.Dst = ctrlPort.AsRemote()
		ctrlPort.Deliver(reset)
		t.Tick()

		Expect(t.State.HeldReqs).To(BeEmpty())
		Expect(t.State.HeldPages).To(BeEmpty())
	})
})
