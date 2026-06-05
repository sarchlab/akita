package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go. The address translator is downstream-dependent:
// a Drain cannot complete until every request it forwarded to its Bottom
// port has been answered, so the Drain test must feed the matching bottom
// responses before the Drain ack can appear.
var _ = Describe("Address Translator control behavior", func() {
	var (
		engine          timing.Engine
		t               *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1
		spec.TopPortBufferSize = 16

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		t = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")

		topPort = t.GetPortByName("Top")
		bottomPort = t.GetPortByName("Bottom")
		translationPort = t.GetPortByName("Translation")
		ctrlPort = t.GetPortByName("Control")

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}
	}

	makeRead := func(addr uint64) mem.ReadReq {
		req := mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	// makeBottomReq builds the read the translator would itself have sent out
	// the Bottom port, with Src = Bottom and Dst = the memory provider, exactly
	// as createTranslatedReq does.
	makeBottomReq := func(addr uint64) mem.ReadReq {
		req := mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = bottomPort.AsRemote()
		req.Dst = messaging.RemotePort("MemPort")
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	// injectInflight populates one in-flight bottom request from a known
	// top-side read and bottom-side read, mirroring the Reset test's direct
	// state fabrication. It returns the bottom-side ReqToBottomID so the test
	// can later feed a matching response that retires the entry.
	injectInflight := func(fromTop, toBottom mem.ReadReq) uint64 {
		t.State.InflightReqToBottom = append(t.State.InflightReqToBottom,
			reqToBottomState{
				ReqFromTopID:    fromTop.ID,
				ReqFromTopSrc:   fromTop.Src,
				ReqFromTopDst:   fromTop.Dst,
				ReqFromTopType:  fmt.Sprintf("%T", fromTop),
				ReqToBottomID:   toBottom.ID,
				ReqToBottomSrc:  toBottom.Src,
				ReqToBottomDst:  toBottom.Dst,
				ReqToBottomType: fmt.Sprintf("%T", toBottom),
			})
		return toBottom.ID
	}

	// feedBottomDataReady delivers a DataReadyRsp on the Bottom port whose
	// RspTo matches an in-flight ReqToBottomID. respond() recognises it, sends a
	// DataReadyRsp out Top, and removes the in-flight entry.
	feedBottomDataReady := func(rspTo uint64) {
		dataReady := mem.DataReadyRsp{}
		dataReady.ID = timing.GetIDGenerator().Generate()
		dataReady.Src = messaging.RemotePort("MemPort")
		dataReady.Dst = bottomPort.AsRemote()
		dataReady.RspTo = rspTo
		dataReady.TrafficBytes = 4
		dataReady.TrafficClass = "mem.DataReadyRsp"
		bottomPort.Deliver(dataReady)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build()
	})

	It("drains all in-flight bottom requests before acking Drain", func() {
		// Fabricate two genuine in-flight bottom requests, exactly like the
		// Reset test does, capturing their ReqToBottomIDs so we can later feed
		// the matching responses.
		readFromTop1 := makeRead(0x10040)
		readToBottom1 := makeBottomReq(0x20040)
		readFromTop2 := makeRead(0x11040)
		readToBottom2 := makeBottomReq(0x21040)

		id1 := injectInflight(readFromTop1, readToBottom1)
		id2 := injectInflight(readFromTop2, readToBottom2)

		// Teeth: in-flight bottom work is genuinely present.
		Expect(t.State.InflightReqToBottom).To(HaveLen(2))

		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		// Negative phase: tick a window WITHOUT feeding any bottom response.
		// The Drain must not ack while in-flight work remains, the component
		// must enter Draining, and the in-flight entries must stay.
		for range 8 {
			t.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					Fail("Drain acked while bottom requests still in flight")
				}
			}
		}
		Expect(t.State.ControlState).To(Equal(control.StateDraining))
		Expect(t.State.InflightReqToBottom).To(HaveLen(2))

		// Positive phase: feed the matching bottom responses. respond() retires
		// each in-flight entry; once none remain, completePendingDrain acks.
		feedBottomDataReady(id1)
		feedBottomDataReady(id2)

		var drainRsp mem.ControlRsp
		drainFound := false
		topResponses := 0
		for i := 0; i < 64 && !drainFound; i++ {
			t.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(mem.DataReadyRsp); ok {
					topResponses++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
					drainFound = true
				}
			}
		}

		Expect(drainFound).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Each retired bottom request produced a Top-side response, and no
		// in-flight state remains by the time the async Drain ack is sent.
		Expect(topResponses).To(Equal(2))
		Expect(t.State.Transactions).To(BeEmpty())
		Expect(t.State.InflightReqToBottom).To(BeEmpty())
		Expect(t.State.ControlState).To(Equal(control.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		t.State.ControlState = control.StatePaused
		topPort.Deliver(makeRead(0x100))

		for range 5 {
			t.Tick()
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out the Bottom port, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(t.State.Transactions).To(BeEmpty())
		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
		Expect(translationPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState control.State) {
			readFromTop := makeRead(0x10040)
			readToBottom := makeBottomReq(0x20040)
			injectInflight(readFromTop, readToBottom)
			Expect(t.State.InflightReqToBottom).ToNot(BeEmpty())

			t.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp mem.ControlRsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				t.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, found = out.(mem.ControlRsp)
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(t.State.Transactions).To(BeEmpty())
			Expect(t.State.InflightReqToBottom).To(BeEmpty())
			Expect(t.State.ControlState).To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		Entry("from Draining", control.StateDraining),
	)
})
