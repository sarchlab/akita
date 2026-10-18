package addresstranslator

import (
	"fmt"

	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go. The address translator is downstream-dependent:
// a Drain cannot complete until every request it forwarded to its Bottom
// port has been answered, so the Drain test must feed the matching bottom
// responses before the Drain ack can appear.
//
//nolint:funlen,gocognit // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestAddressTranslatorControlBehavior(t *testing.T) {
	var (
		engine          timing.Engine
		sim             timing.Simulation
		translator      *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("MemPort")}
		spec.Translation.Destinations = []messaging.RemotePort{messaging.RemotePort("TranslationProvider")}

		translator = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("AddressTranslator")
		tPorts := makePorts("AddressTranslator", 16)
		translator.BindPort("Top", tPorts.Top)
		translator.BindPort("Bottom", tPorts.Bottom)
		translator.BindPort("Translation", tPorts.Translation)
		translator.BindPort("Control", tPorts.Control)

		topPort = translator.Ports.Top
		bottomPort = translator.Ports.Bottom
		translationPort = translator.Ports.Translation
		ctrlPort = translator.Ports.Control

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
		translator.State.InflightReqToBottom = append(translator.State.InflightReqToBottom,
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

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build()
	}

	t.Run("drains all in-flight bottom requests before acking Drain", func(t *testing.T) {
		setup1(t)

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
		require.Len(t, translator.State.InflightReqToBottom, 2)

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// Negative phase: tick a window WITHOUT feeding any bottom response.
		// The Drain must not ack while in-flight work remains, the component
		// must enter Draining, and the in-flight entries must stay.
		for range 8 {
			modelingtest.Tick(translator)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					t.Fatal("Drain acked while bottom requests still in flight")
				}
			}
		}
		require.Equal(t, memcontrolprotocol.StateDraining, translator.State.ControlState)
		require.Len(t, translator.State.InflightReqToBottom, 2)

		// Positive phase: feed the matching bottom responses. respond() retires
		// each in-flight entry; once none remain, completePendingDrain acks.
		feedBottomDataReady(id1)
		feedBottomDataReady(id2)

		var drainRsp messaging.Msg
		drainFound := false
		topResponses := 0
		for i := 0; i < 64 && !drainFound; i++ {
			modelingtest.Tick(translator)
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

		require.True(t, drainFound)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		// Each retired bottom request produced a Top-side response, and no
		// in-flight state remains by the time the async Drain ack is sent.
		require.Equal(t, 2, topResponses)
		require.Empty(t, translator.State.Transactions)
		require.Empty(t, translator.State.InflightReqToBottom)
		require.Equal(t, memcontrolprotocol.StatePaused, translator.State.ControlState)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		translator.State.ControlState = memcontrolprotocol.StatePaused
		topPort.Deliver(makeRead(0x100))

		for range 5 {
			modelingtest.Tick(translator)
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out the Bottom port, while paused.
		_, present0 := topPort.PeekIncoming()
		require.True(t, present0)
		require.Empty(t, translator.State.Transactions)
		_, present1 := bottomPort.RetrieveOutgoing()
		require.False(t, present1)
		_, present2 := translationPort.RetrieveOutgoing()
		require.False(t, present2)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState memcontrolprotocol.State) {
			t.Helper()

			setup1(t)

			readFromTop := makeRead(0x10040)
			readToBottom := makeBottomReq(0x20040)
			injectInflight(readFromTop, readToBottom)
			require.NotEmpty(t, translator.State.InflightReqToBottom)

			translator.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(translator)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, found)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			require.Empty(t, translator.State.Transactions)
			require.Empty(t, translator.State.InflightReqToBottom)
			require.Equal(t, memcontrolprotocol.StateEnabled, translator.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, memcontrolprotocol.StateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, memcontrolprotocol.StatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent: completePendingDrain acks the Drain.
		// Control commands are serialized with no preemption, so a Reset queued
		// behind the drain is serviced only after the Drain acks.
		translator.State.ControlState = memcontrolprotocol.StateDraining
		translator.State.CurrentCmdID = 999
		translator.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		translator.State.Transactions = nil
		translator.State.InflightReqToBottom = nil

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(translator)
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

		require.Len(t, rsps, 2)
		require.Equal(t, memcontrolprotocol.CmdDrain, rsps[0].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, uint64(999), rsps[0].RspTo)
		require.Equal(t, memcontrolprotocol.CmdReset, rsps[1].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, reset.ID, rsps[1].RspTo)
		require.Equal(t, memcontrolprotocol.StateEnabled, translator.State.ControlState)
	})
}
