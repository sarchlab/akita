package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestTLB(t *testing.T) {
	var (
		engine      timing.Engine
		sim         *testsim.Simulation
		tlbComp     *Comp
		tlbMW       *tlbMiddleware
		tlbCtrlMW   *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
		remotePort  = messaging.RemotePort("RemotePort")
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		spec := DefaultSpec()
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		spec.Bottom.Destinations = []messaging.RemotePort{remotePort}
		tlbComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("TLB")
		tlbCompPorts := defaultPorts(sim, "TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)
		plugNoopConn(tlbComp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		topPort = tlbComp.Ports.Top
		bottomPort = tlbComp.Ports.Bottom
		controlPort = tlbComp.Ports.Control

		tlbMW = tlbComp.Middlewares.TLB
		tlbCtrlMW = tlbComp.Middlewares.Ctrl
	}

	t.Run("should do nothing if there is no req in TopPort", func(t *testing.T) {
		setup1(t)

		madeProgress := tlbMW.insertIntoPipeline()

		require.False(t, madeProgress)
	})

	t.Run("should insert req into pipeline when topPort has req", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    uint64(0x100),
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),
		}

		topPort.Deliver(req)

		madeProgress := tlbMW.insertIntoPipeline()

		require.True(t, madeProgress)
	})

	t.Run("hit", func(t *testing.T) {
		var (
			req messaging.Msg
		)

		setup2 := func(t *testing.T) {
			t.Helper()

			// Set up a page in the TLB state
			page := vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			next := &tlbComp.State
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			req = messaging.Msg{Payload: addrtransprotocol.Req{
				PID:      1,
				VAddr:    uint64(0x100),
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
			}
		}

		t.Run("should respond to top", func(t *testing.T) {
			setup1(t)
			setup2(t)

			madeProgress := tlbMW.lookup(pipelineTLBReqState{Msg: req})

			require.True(t, madeProgress)
			rsp, _ := topPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: addrtransprotocol.Rsp{}}, rsp)
		})
	})

	t.Run("miss", func(t *testing.T) {
		var (
			req messaging.Msg
		)

		setup3 := func(t *testing.T) {
			t.Helper()

			// Set up a page with Valid=false to trigger miss
			page := vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: false,
			}
			next := &tlbComp.State
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			req = messaging.Msg{Payload: addrtransprotocol.Req{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
			}
		}

		t.Run("should fetch from bottom and add entry to MSHR", func(t *testing.T) {
			setup1(t)
			setup3(t)

			madeProgress := tlbMW.lookup(pipelineTLBReqState{Msg: req})

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.Equal(t, true, mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100)))

			sent, _ := bottomPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: addrtransprotocol.Req{}}, sent)
			sentMsg := sent
			require.Equal(t, uint64(0x100), sentMsg.Payload.(addrtransprotocol.Req).VAddr)
			require.Equal(t, vm.PID(1), sentMsg.Payload.(addrtransprotocol.Req).PID)
			require.Equal(t, uint64(1), sentMsg.Payload.(addrtransprotocol.Req).DeviceID)
			require.Equal(t, remotePort, sentMsg.Dst)
		})
	})

	t.Run("parse bottom", func(t *testing.T) {
		var (
			req         messaging.Msg
			fetchBottom messaging.Msg
			page        vm.Page
			rsp         messaging.Msg
		)

		setup4 := func(t *testing.T) {
			t.Helper()

			req = messaging.Msg{Payload: addrtransprotocol.Req{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
			}

			fetchBottom = messaging.Msg{Payload: addrtransprotocol.Req{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),
			}

			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = messaging.Msg{Payload: addrtransprotocol.Rsp{
				Page: page,
			},
				ID:    sim.NewID(),
				Src:   messaging.RemotePort("Agent"),
				Dst:   bottomPort.AsRemote(),
				RspTo: fetchBottom.ID,
			}
		}

		t.Run("should do nothing if no return", func(t *testing.T) {
			setup1(t)
			setup4(t)

			madeProgress := tlbMW.parseBottom()

			require.False(t, madeProgress)
		})

		t.Run("should stall if the TLB is responding to an MSHR entry", func(t *testing.T) {
			setup1(t)
			setup4(t)

			// Set up responding MSHR entry
			next := &tlbComp.State
			next.HasRespondingMSHR = true
			next.RespondingMSHRData = mshrEntryState{
				PID:      1,
				VAddr:    0x100,
				Requests: []pipelineTLBReqState{{Msg: req}},
			}
			// Also add the MSHR entry
			next.MSHREntries, _ = mshrAdd(next.MSHREntries, 4, 1, 0x100)

			madeProgress := tlbMW.parseBottom()

			require.False(t, madeProgress)
		})

		t.Run("should parse respond from bottom", func(t *testing.T) {
			setup1(t)
			setup4(t)

			bottomPort.Deliver(rsp)

			// Add MSHR entry
			next := &tlbComp.State
			next.MSHREntries, _ = mshrAdd(next.MSHREntries, 4, 1, 0x100)
			idx, _ := mshrGetEntry(next.MSHREntries, 1, 0x100)
			next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, pipelineTLBReqState{Msg: req})
			next.MSHREntries[idx].HasReqToBottom = true
			next.MSHREntries[idx].ReqToBottom = fetchBottom

			madeProgress := tlbMW.parseBottom()

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.True(t, nextState.HasRespondingMSHR)
			require.Equal(t, false, mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100)))
		})

		t.Run("should respond", func(t *testing.T) {
			setup1(t)
			setup4(t)

			next := &tlbComp.State
			next.HasRespondingMSHR = true
			next.RespondingMSHRData = mshrEntryState{
				PID:      1,
				VAddr:    0x100,
				Requests: []pipelineTLBReqState{{Msg: req}},
			}

			madeProgress := tlbMW.respondMSHREntry()

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.Len(t, nextState.RespondingMSHRData.Requests, 0)
			require.False(t, nextState.HasRespondingMSHR)

			rsp, _ := topPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: addrtransprotocol.Rsp{}}, rsp)
		})
	})

	t.Run("flush related handling", func(t *testing.T) {
		t.Run("should do nothing if no req", func(t *testing.T) {
			setup1(t)

			madeProgress := tlbCtrlMW.handleIncomingCommands()
			require.False(t, madeProgress)
		})

		t.Run("should invalidate a matching entry when paused", func(t *testing.T) {
			setup1(t)

			next := &tlbComp.State
			next.TLBState = tlbStatePause

			page := vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: true,
			}
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command:   memcontrolprotocol.CmdInvalidate,
				Addresses: []uint64{0x1000},
				PID:       1,
			},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: controlPort.AsRemote(),
			}

			controlPort.Deliver(invReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()
			require.True(t, madeProgress)

			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			require.False(t, found && gotPage.Valid)

			rspMsg, _ := controlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, rspMsg)
			rsp := rspMsg
			require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		})

		t.Run("should reject Invalidate while enabled", func(t *testing.T) {
			setup1(t)

			next := &tlbComp.State
			next.TLBState = tlbStateEnable

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: controlPort.AsRemote(),
			}

			controlPort.Deliver(invReq)
			require.True(t, tlbCtrlMW.handleIncomingCommands())

			rspMsg, _ := controlPort.RetrieveOutgoing()
			rsp := rspMsg
			require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, memcontrolprotocol.ErrMustBePausedOrDrained, rsp.Payload.(memcontrolprotocol.Rsp).Error)
		})

		t.Run("invalidates only entries matching the PID filter", func(t *testing.T) {
			setup1(t)

			next := &tlbComp.State
			next.TLBState = tlbStatePause

			pageA := vm.Page{PID: 1, VAddr: 0x1000, Valid: true}
			pageB := vm.Page{PID: 2, VAddr: 0x2000, Valid: true}
			setUpdate(&next.Sets[0], 0, pageA)
			setVisit(&next.Sets[0], 0)
			setUpdate(&next.Sets[0], 1, pageB)
			setVisit(&next.Sets[0], 1)

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: controlPort.AsRemote(),
			}

			controlPort.Deliver(invReq)

			require.True(t, tlbCtrlMW.handleIncomingCommands())

			// Only the PID-1 entry is dropped; the PID-2 entry survives.
			require.False(t, next.Sets[0].Blocks[0].Page.Valid)
			require.True(t, next.Sets[0].Blocks[1].Page.Valid)

			rspValue, _ := controlPort.RetrieveOutgoing()

			rsp := rspValue
			require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		})

		t.Run("clears cached entries on Reset", func(t *testing.T) {
			setup1(t)

			next := &tlbComp.State
			page := vm.Page{PID: 1, VAddr: 0x1000, Valid: true}
			setUpdate(&next.Sets[0], 0, page)
			setVisit(&next.Sets[0], 0)

			resetReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: controlPort.AsRemote(),
			}

			controlPort.Deliver(resetReq)

			require.True(t, tlbCtrlMW.handleIncomingCommands())

			// Reset returns the TLB to its freshly-built empty state.
			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			require.False(t, found && gotPage.Valid)

			rspValue, _ := controlPort.RetrieveOutgoing()

			rsp := rspValue
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		})

		t.Run("should handle restart request", func(t *testing.T) {
			setup1(t)

			restartReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),
				Dst: controlPort.AsRemote(),
			}

			controlPort.Deliver(restartReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			require.True(t, madeProgress)

			rsp, _ := controlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, rsp)
		})
	})

	t.Run("other control signals", func(t *testing.T) {
		t.Run("should handle pause ctrl msg", func(t *testing.T) {
			setup1(t)

			pauseMsg := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
			}

			controlPort.Deliver(pauseMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.Equal(t, tlbStatePause, nextState.TLBState)
		})

		t.Run("should handle enable ctrl msg after pause", func(t *testing.T) {
			setup1(t)

			pause := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
			}

			controlPort.Deliver(pause)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.Equal(t, tlbStatePause, nextState.TLBState)

			// Drain the Pause ack so the next ack has room in the
			// single-slot control port outgoing buffer.
			value0, present0 := controlPort.RetrieveOutgoing()
			require.True(t, present0)
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, value0)

			enable := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdEnable,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
			}

			controlPort.Deliver(enable)

			madeProgress = tlbCtrlMW.handleIncomingCommands()
			require.True(t, madeProgress)
			nextState = &tlbComp.State
			require.Equal(t, tlbStateEnable, nextState.TLBState)
		})

		t.Run("should handle drain ctrl msg", func(t *testing.T) {
			setup1(t)

			drainMsg := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdDrain,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
			}

			controlPort.Deliver(drainMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			require.True(t, madeProgress)
			nextState := &tlbComp.State
			require.Equal(t, tlbStateDrain, nextState.TLBState)

			madeProgress = tlbMW.handleDrain()
			require.False(t, madeProgress)
			nextState = &tlbComp.State
			require.Equal(t, tlbStatePause, nextState.TLBState)
		})
	})
}

func TestTLBIntegration(t *testing.T) {
	var (
		engine     timing.Engine
		sim        *testsim.Simulation
		tlbComp    *Comp
		lowModule  *idealEndpoint
		agent      *idealEndpoint
		connection messaging.Connection
		page       vm.Page
	)

	setup5 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		connection = makeDirectConnection(sim)

		lowModule = newIdealEndpoint(sim, "LowModule")
		agent = newIdealEndpoint(sim, "Agent")

		routingSpec2 := DefaultSpec()

		routingSpec2.Bottom.Destinations = []messaging.RemotePort{lowModule.port.AsRemote()}
		tlbComp = modeling.MakeBuilder[*Comp](routingSpec2).
			WithSimulation(sim).
			Build("TLB")
		tlbCompPorts := defaultPorts(sim, "TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)

		connection.BindPort(agent.port)
		connection.BindPort(lowModule.port)
		connection.BindPort(tlbComp.Ports.Top)
		connection.BindPort(tlbComp.Ports.Bottom)
		connection.BindPort(tlbComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		page = vm.Page{
			PID:   1,
			VAddr: 0x1000,
			PAddr: 0x2000,
			Valid: true,
		}

		// lowModule answers every translation request with the page.
		lowModule.onDeliver = func(msg messaging.Msg) {
			translationReq := msg
			rsp := messaging.Msg{Payload: addrtransprotocol.Rsp{Page: page},
				ID:    sim.NewID(),
				Src:   lowModule.port.AsRemote(),
				Dst:   translationReq.Src,
				RspTo: translationReq.ID,
			}

			lowModule.port.Send(rsp)
		}
	}

	t.Run("should do tlb miss", func(t *testing.T) {
		setup5(t)

		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),
		}

		agent.port.Send(req)

		require.NoError(t, engine.Run())

		rsp := agent.lastDelivered
		require.Equal(t, page, rsp.Payload.(addrtransprotocol.Rsp).Page)
	})

	t.Run("should have faster hit than miss", func(t *testing.T) {
		setup5(t)

		time1 := engine.CurrentTime()
		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),
		}

		agent.port.Send(req)

		require.NoError(t, engine.Run())

		rsp := agent.lastDelivered
		require.Equal(t, page, rsp.Payload.(addrtransprotocol.Rsp).Page)

		time2 := engine.CurrentTime()

		req2 := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),
		}

		agent.port.Send(req2)

		require.NoError(t, engine.Run())

		rsp = agent.lastDelivered
		require.Equal(t, page, rsp.Payload.(addrtransprotocol.Rsp).Page)

		time3 := engine.CurrentTime()

		require.Less(t, time3-time2, time2-time1)
	})
}
