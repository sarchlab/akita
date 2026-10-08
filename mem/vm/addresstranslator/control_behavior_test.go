package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
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
		sim             timing.Simulation
		t               *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
	)

	build := func() {
		spec := Definition.DefaultSpec
		spec.Log2PageSize = 12
		spec.Freq = 1

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		t = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")
		tPorts := makePorts("AddressTranslator", 16)
		t.BindPort("Top", tPorts.Top)
		t.BindPort("Bottom", tPorts.Bottom)
		t.BindPort("Translation", tPorts.Translation)
		t.BindPort("Control", tPorts.Control)

		topPort = t.Ports.Top
		bottomPort = t.Ports.Bottom
		translationPort = t.Ports.Translation
		ctrlPort = t.Ports.Control

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Ctrl"),
			Dst:          ctrlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		return req
	}

	// makeBottomReq builds the read the translator would itself have sent out
	// the Bottom port, with Src = Bottom and Dst = the memory provider, exactly
	// as createTranslatedReq does.
	makeBottomReq := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: bottomPort.AsRemote(),
			Dst: messaging.RemotePort("MemPort"),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	// injectInflight populates one in-flight bottom request from a known
	// top-side read and bottom-side read, mirroring the Reset test's direct
	// state fabrication. It returns the bottom-side ReqToBottomID so the test
	// can later feed a matching response that retires the entry.
	injectInflight := func(fromTop, toBottom messaging.Msg) uint64 {
		t.State.InflightReqToBottom = append(t.State.InflightReqToBottom,
			reqToBottomState{
				ReqFromTopID:    fromTop.ID,
				ReqFromTopSrc:   fromTop.Src,
				ReqFromTopDst:   fromTop.Dst,
				ReqFromTopType:  fmt.Sprintf("%T", fromTop.Payload),
				ReqToBottomID:   toBottom.ID,
				ReqToBottomSrc:  toBottom.Src,
				ReqToBottomDst:  toBottom.Dst,
				ReqToBottomType: fmt.Sprintf("%T", toBottom.Payload),
			})
		return toBottom.ID
	}

	// feedBottomDataReady delivers a DataReadyRsp on the Bottom port whose
	// RspTo matches an in-flight ReqToBottomID. respond() recognises it, sends a
	// DataReadyRsp out Top, and removes the in-flight entry.
	feedBottomDataReady := func(rspTo uint64) {
		dataReady := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("MemPort"),
			Dst:          bottomPort.AsRemote(),
			RspTo:        rspTo,
			TrafficBytes: 4,
			TrafficClass: "memprotocol.DataReadyRsp"}

		bottomPort.Deliver(dataReady)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
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

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// Negative phase: tick a window WITHOUT feeding any bottom response.
		// The Drain must not ack while in-flight work remains, the component
		// must enter Draining, and the in-flight entries must stay.
		for range 8 {
			modelingtest.Tick(t)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					Fail("Drain acked while bottom requests still in flight")
				}

			}
		}
		Expect(t.State.ControlState).To(Equal(memcontrolprotocol.StateDraining))
		Expect(t.State.InflightReqToBottom).To(HaveLen(2))

		// Positive phase: feed the matching bottom responses. respond() retires
		// each in-flight entry; once none remain, completePendingDrain acks.
		feedBottomDataReady(id1)
		feedBottomDataReady(id2)

		var drainRsp messaging.Msg
		drainFound := false
		topResponses := 0
		for i := 0; i < 64 && !drainFound; i++ {
			modelingtest.Tick(t)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
					topResponses++
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					drainFound = true
				}

			}
		}

		Expect(drainFound).To(BeTrue())
		Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Each retired bottom request produced a Top-side response, and no
		// in-flight state remains by the time the async Drain ack is sent.
		Expect(topResponses).To(Equal(2))
		Expect(t.State.Transactions).To(BeEmpty())
		Expect(t.State.InflightReqToBottom).To(BeEmpty())
		Expect(t.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		t.State.ControlState = memcontrolprotocol.StatePaused
		topPort.Deliver(makeRead(0x100))

		for range 5 {
			modelingtest.Tick(t)
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out the Bottom port, while paused.
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeTrue())
		Expect(t.State.Transactions).To(BeEmpty())
		_, present1 := bottomPort.RetrieveOutgoing()
		Expect(present1).To(BeFalse())
		_, present2 := translationPort.RetrieveOutgoing()
		Expect(present2).To(BeFalse())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState memcontrolprotocol.State) {
			readFromTop := makeRead(0x10040)
			readToBottom := makeBottomReq(0x20040)
			injectInflight(readFromTop, readToBottom)
			Expect(t.State.InflightReqToBottom).ToNot(BeEmpty())

			t.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(t)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(t.State.Transactions).To(BeEmpty())
			Expect(t.State.InflightReqToBottom).To(BeEmpty())
			Expect(t.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
		},
		Entry("from Enabled", memcontrolprotocol.StateEnabled),
		Entry("from Paused", memcontrolprotocol.StatePaused),
		// The Draining case is covered separately: under strict serialization a
		// Reset queued behind an in-progress Drain is serviced only after the
		// Drain acks, so it gets its own It test below.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining and already quiescent: completePendingDrain acks the Drain.
		// Control commands are serialized with no preemption, so a Reset queued
		// behind the drain is serviced only after the Drain acks.
		t.State.ControlState = memcontrolprotocol.StateDraining
		t.State.CurrentCmdID = 999
		t.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		t.State.Transactions = nil
		t.State.InflightReqToBottom = nil

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(t)
			for {
				out, ok := ctrlPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memcontrolprotocol.Rsp); ok {

					rsps = append(rsps, out)
				}
			}
		}

		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(t.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})
})
