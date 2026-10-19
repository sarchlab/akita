package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// noopConn is a minimal messaging.Connection used to drive the reorder
// buffer's real ports in isolation. The ROB owns its ports, so tests feed
// requests via Deliver and consume sent messages via RetrieveOutgoing; the
// port still needs a connection so its send/retrieve notifications have
// somewhere to land.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(_ messaging.Port)      {}

func TestReorderBuffer(t *testing.T) {
	const (
		topRemote        = messaging.RemotePort("Agent")
		bottomUnitRemote = messaging.RemotePort("BottomUnit")
	)

	var (
		engine     timing.Engine
		sim        timing.Simulation
		rob        *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port

		topBufSize    int
		bottomBufSize int
		ctrlBufSize   int
	)

	build := func(spec Spec) {
		port := func(name string, bufSize int) messaging.Port {
			return twowaybuffered.NewPort(bufSize, bufSize)
		}

		topPort = port("Top", topBufSize)
		bottomPort = port("Bottom", bottomBufSize)
		ctrlPort = port("Control", ctrlBufSize)

		rob = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("Rob")

		rob.BindPort("Top", topPort)
		rob.BindPort("Bottom", bottomPort)
		rob.BindPort("Control", ctrlPort)

		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
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
			AccessByteSize: 4,
		},
			ID:           sim.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
		}

		return req
	}

	makeWrite := func(addr uint64, data []byte) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: addr,
			Data:    data,
		},
			ID:           sim.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficBytes: len(data) + 12,
		}

		return req
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		spec := DefaultSpec()
		spec.BufferSize = 4
		spec.NumReqPerCycle = 2
		spec.Bottom = bottomUnitRemote
		topBufSize = 4
		bottomBufSize = 4
		ctrlBufSize = 2
		build(spec)
	}

	t.Run("top-down", func(t *testing.T) {
		t.Run("forwards a read to the bottom and records the transaction", func(t *testing.T) {
			setup1(t)

			req := makeRead(0)
			topPort.Deliver(req)

			progress := modelingtest.Tick(rob)

			require.True(t, progress)
			require.Len(t, rob.State.Transactions, 1)
			require.True(t, rob.State.Transactions[0].IsRead)
			require.Equal(t, req.ID, rob.State.Transactions[0].ReqFromTopID)
			require.Equal(t, topRemote, rob.State.Transactions[0].ReqFromTopSrc)

			sent, _ := bottomPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memprotocol.ReadReq{}}, sent)
			shadow := sent
			require.Equal(t, bottomPort.AsRemote(), shadow.Src)
			require.Equal(t, bottomUnitRemote, shadow.Dst)
			require.Equal(t, uint64(0), shadow.Payload.(memprotocol.ReadReq).Address)
			require.Equal(t, uint64(4), shadow.Payload.(memprotocol.ReadReq).AccessByteSize)
			require.Equal(t, rob.State.Transactions[0].ReqToBottomID, shadow.ID)
			require.NotEqual(t, req.ID, shadow.ID)
		})

		t.Run("forwards a write to the bottom and records the transaction", func(t *testing.T) {
			setup1(t)

			req := makeWrite(64, []byte{1, 2, 3, 4})
			topPort.Deliver(req)

			progress := modelingtest.Tick(rob)

			require.True(t, progress)
			require.Len(t, rob.State.Transactions, 1)
			require.False(t, rob.State.Transactions[0].IsRead)

			sent, _ := bottomPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memprotocol.WriteReq{}}, sent)
			shadow := sent
			require.Equal(t, []byte{1, 2, 3, 4}, shadow.Payload.(memprotocol.WriteReq).Data)
			require.Equal(t, bottomUnitRemote, shadow.Dst)
		})

		t.Run("stalls when the buffer is full", func(t *testing.T) {
			setup1(t)

			spec := rob.Spec
			for i := 0; i < spec.BufferSize; i++ {
				rob.State.Transactions = append(rob.State.Transactions,
					transactionState{IsRead: true})
			}

			topPort.Deliver(makeRead(0))

			progress := modelingtest.Tick(rob)

			require.False(t, progress)
			require.Len(t, rob.State.Transactions, spec.BufferSize)
			_, present0 := topPort.PeekIncoming()
			require.True(t, present0)
		})

		t.Run("stalls when the bottom port is full", func(t *testing.T) {
			setup1(t)

			topPort.Deliver(makeRead(0))

			// Fill the bottom outgoing buffer so Send fails.
			for i := 0; i < bottomBufSize; i++ {
				filler := messaging.Msg{Payload: memprotocol.ReadReq{Address: uint64(i)},
					ID:  sim.NewID(),
					Src: bottomPort.AsRemote(),
					Dst: bottomUnitRemote,
				}

				require.True(t, bottomPort.CanSend())
				bottomPort.Send(filler)
			}

			progress := modelingtest.Tick(rob)

			require.False(t, progress)
			require.Empty(t, rob.State.Transactions)
			_, present1 := topPort.PeekIncoming()
			require.True(t, present1)
		})

		t.Run("panics on unsupported top-port traffic", func(t *testing.T) {
			setup1(t)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID:  sim.NewID(),
				Src: topRemote,
				Dst: topPort.AsRemote(),
			}

			topPort.Deliver(req)

			require.Panics(t, func() { modelingtest.Tick(rob) })
		})
	})

	t.Run("parse bottom", func(t *testing.T) {
		t.Run("attaches the bottom response to the matching transaction", func(t *testing.T) {
			setup1(t)

			req := makeWrite(0, []byte{0xAA})
			topPort.Deliver(req)
			modelingtest.Tick(rob) // forward to bottom

			require.Len(t, rob.State.Transactions, 1)
			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: shadowID,
			}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob)

			// The response is recorded on this tick; bottomUp will drain
			// the head on the following tick (hardware pipeline ordering).
			require.Len(t, rob.State.Transactions, 1)
			require.True(t, rob.State.Transactions[0].HasRsp)
		})

		t.Run("ignores a response that does not match any transaction", func(t *testing.T) {
			setup1(t)

			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: 999999,
			}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob)

			_, present2 := bottomPort.PeekIncoming()
			require.False(t, present2)
		})

		t.Run("drops unsupported bottom-port traffic", func(t *testing.T) {
			setup1(t)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID:  sim.NewID(),
				Src: bottomUnitRemote,
				Dst: bottomPort.AsRemote(),
			}

			bottomPort.Deliver(req)

			progress := modelingtest.Tick(rob)

			require.True(t, progress)
			_, present3 := bottomPort.PeekIncoming()
			require.False(t, present3)
			require.Empty(t, rob.State.Transactions)
		})
	})

	t.Run("bottom up", func(t *testing.T) {
		t.Run("forwards the head response to the top once it is ready", func(t *testing.T) {
			setup1(t)

			req := makeRead(0)
			topPort.Deliver(req)
			modelingtest.Tick(rob)
			shadowSent, _ := bottomPort.RetrieveOutgoing()
			require.NotNil(t, shadowSent)

			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0xDE, 0xAD}},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: shadowID,
			}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob) // parseBottom records the response
			modelingtest.Tick(rob) // bottomUp drains the head

			require.Empty(t, rob.State.Transactions)

			topOut, _ := topPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memprotocol.DataReadyRsp{}}, topOut)
			data := topOut
			require.Equal(t, []byte{0xDE, 0xAD}, data.Payload.(memprotocol.DataReadyRsp).Data)
			require.Equal(t, req.ID, data.RspTo)
			require.Equal(t, topRemote, data.Dst)
			require.Equal(t, topPort.AsRemote(), data.Src)
		})

		t.Run("preserves FIFO order across multiple in-flight transactions", func(t *testing.T) {
			setup1(t)

			r1 := makeRead(0x100)
			r2 := makeRead(0x200)
			topPort.Deliver(r1)
			topPort.Deliver(r2)

			// Tick once: both requests forward to the bottom in the same
			// cycle (NumReqPerCycle=2).
			modelingtest.Tick(rob)
			require.Len(t, rob.State.Transactions, 2)
			_, present4 := bottomPort.RetrieveOutgoing()
			require.True(t, present4)
			_, present5 := bottomPort.RetrieveOutgoing()
			require.True(t, present5)
			shadow1 := rob.State.Transactions[0].ReqToBottomID
			shadow2 := rob.State.Transactions[1].ReqToBottomID

			// Deliver the second response first; the head must still wait
			// since its response has not arrived yet.
			rsp2 := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x22}},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: shadow2,
			}

			bottomPort.Deliver(rsp2)

			modelingtest.Tick(rob) // parseBottom records rsp2
			modelingtest.Tick(rob) // bottomUp finds head not ready, makes no progress

			require.Len(t, rob.State.Transactions, 2)
			require.False(t, rob.State.Transactions[0].HasRsp)
			require.True(t, rob.State.Transactions[1].HasRsp)
			_, present6 := topPort.RetrieveOutgoing()
			require.False(t, present6)
			// Now deliver the response for the head; both should drain in
			// order on subsequent ticks.
			rsp1 := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x11}},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: shadow1,
			}

			bottomPort.Deliver(rsp1)

			modelingtest.Tick(rob) // parseBottom records rsp1
			modelingtest.Tick(rob) // bottomUp drains both (NumReqPerCycle=2)

			require.Empty(t, rob.State.Transactions)

			out1, _ := topPort.RetrieveOutgoing()
			out2, _ := topPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memprotocol.DataReadyRsp{}}, out1)
			require.IsType(t, messaging.Msg{Payload: memprotocol.DataReadyRsp{}}, out2)
			require.Equal(t, []byte{0x11}, out1.Payload.(memprotocol.DataReadyRsp).Data)
			require.Equal(t, []byte{0x22}, out2.Payload.(memprotocol.DataReadyRsp).Data)
			require.Equal(t, r1.ID, out1.RspTo)
			require.Equal(t, r2.ID, out2.RspTo)
		})

		t.Run("stalls when the top port cannot accept the response", func(t *testing.T) {
			setup1(t)

			req := makeRead(0)
			topPort.Deliver(req)
			modelingtest.Tick(rob)
			bottomPort.RetrieveOutgoing()

			shadowID := rob.State.Transactions[0].ReqToBottomID
			rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x1}},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: shadowID,
			}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob) // parseBottom records the response

			// Fill the top outgoing buffer so the next bottomUp Send fails.
			for i := 0; i < topBufSize; i++ {
				filler := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{byte(i)}},
					ID:  sim.NewID(),
					Src: topPort.AsRemote(),
					Dst: topRemote,
				}

				require.True(t, topPort.CanSend())
				topPort.Send(filler)
			}

			modelingtest.Tick(rob)

			// The head should still be present and marked HasRsp because
			// bottomUp could not enqueue the outgoing response.
			require.Len(t, rob.State.Transactions, 1)
			require.True(t, rob.State.Transactions[0].HasRsp)
		})
	})

	t.Run("control", func(t *testing.T) {
		t.Run("drops in-flight transactions and pauses on CmdReset", func(t *testing.T) {
			setup1(t)

			topPort.Deliver(makeRead(0))
			modelingtest.Tick(rob)
			bottomPort.RetrieveOutgoing()
			require.Len(t, rob.State.Transactions, 1)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdReset,
			},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Cmd"),
				Dst: ctrlPort.AsRemote(),
			}

			ctrlPort.Deliver(req)

			modelingtest.Tick(rob)

			require.Empty(t, rob.State.Transactions)
			require.Equal(t, controlStateEnabled, rob.State.ControlState)

			ack, _ := ctrlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, ack)
			rsp := ack
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, req.ID, rsp.RspTo)
		})

		t.Run("freezes the data pipeline while paused", func(t *testing.T) {
			setup1(t)

			rob.State.ControlState = controlStatePaused
			topPort.Deliver(makeRead(0))

			progress := modelingtest.Tick(rob)

			require.False(t, progress)
			require.Empty(t, rob.State.Transactions)
			_, present7 := topPort.PeekIncoming()
			require.True(t, present7)
		})

		t.Run("resumes on CmdEnable, draining incoming traffic", func(t *testing.T) {
			setup1(t)

			rob.State.ControlState = controlStatePaused

			// Stale traffic that should be cleared on resume.
			topPort.Deliver(makeRead(0))
			stray := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0xFF}},
				ID:    sim.NewID(),
				Src:   bottomUnitRemote,
				Dst:   bottomPort.AsRemote(),
				RspTo: 0xDEAD,
			}

			bottomPort.Deliver(stray)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdEnable},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Cmd"),
				Dst: ctrlPort.AsRemote(),
			}

			ctrlPort.Deliver(req)

			modelingtest.Tick(rob)

			require.Equal(t, controlStateEnabled, rob.State.ControlState)
			_, present8 := topPort.PeekIncoming()
			require.False(t, present8)
			_, present9 := bottomPort.PeekIncoming()
			require.False(t, present9)
			ack, _ := ctrlPort.RetrieveOutgoing()
			require.IsType(t, messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, ack)
			require.Equal(t, memcontrolprotocol.CmdEnable, ack.Payload.(memcontrolprotocol.Rsp).Command)
		})

		makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Cmd"),
				Dst: ctrlPort.AsRemote(),
			}

			return req
		}

		t.Run("acks Drain only after in-flight transactions retire", func(t *testing.T) {
			setup1(t)

			const n = 2
			for i := range n {
				topPort.Deliver(makeRead(uint64(i * 0x100)))
			}
			modelingtest.Tick(rob) // forward both to the bottom (NumReqPerCycle=2)
			require.Len(t, rob.State.Transactions, n)

			shadowIDs := make([]uint64, 0, n)
			for i := range rob.State.Transactions {
				shadowIDs = append(shadowIDs,
					rob.State.Transactions[i].ReqToBottomID)
			}
			for {
				if _, ok := bottomPort.RetrieveOutgoing(); !ok {
					break
				}
			}

			drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
			ctrlPort.Deliver(drain)

			// While transactions are still in flight (no bottom responses
			// fed yet), Drain must stay pending and emit no ack.
			for range 5 {
				modelingtest.Tick(rob)
				require.Equal(t, controlStateDraining, rob.State.ControlState)
				_, present10 := ctrlPort.RetrieveOutgoing()
				require.False(t, present10)
			}
			require.Len(t, rob.State.Transactions, n)

			// Now let the in-flight reads complete.
			for _, id := range shadowIDs {
				rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x1}},
					ID:    sim.NewID(),
					Src:   bottomUnitRemote,
					Dst:   bottomPort.AsRemote(),
					RspTo: id,
				}

				bottomPort.Deliver(rsp)
			}

			completed := 0
			var drainRsp messaging.Msg
			drainFound := false
			for i := 0; i < 64 && !drainFound; i++ {
				modelingtest.Tick(rob)
				for {
					out, ok := topPort.RetrieveOutgoing()
					if !ok {
						break
					}
					if _, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
						completed++
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
			// All in-flight reads finished before the async Drain ack.
			require.Equal(t, n, completed)
			require.Empty(t, rob.State.Transactions)
			require.Equal(t, controlStatePaused, rob.State.ControlState)
		})

		t.Run("Reset wipes in-flight transactions from any state", func(t *testing.T) {
			runCase := func(t *testing.T, startState controlState) {
				t.Helper()

				setup1(t)

				topPort.Deliver(makeRead(0))
				modelingtest.Tick(rob)
				require.Len(t, rob.State.Transactions, 1)

				rob.State.ControlState = startState

				reset := makeCtrlReq(memcontrolprotocol.CmdReset)
				ctrlPort.Deliver(reset)

				var rsp messaging.Msg
				found := false
				for i := 0; i < 64 && !found; i++ {
					modelingtest.Tick(rob)
					if out, ok := ctrlPort.RetrieveOutgoing(); ok {
						_, found = out.Payload.(memcontrolprotocol.Rsp)
						rsp = out
					}
				}

				require.True(t, found)
				require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
				require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
				require.Equal(t, reset.ID, rsp.RspTo)
				require.Empty(t, rob.State.Transactions)
				require.Equal(t, controlStateEnabled, rob.State.ControlState)
			}
			t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
			t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
		})

		t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
			setup1(t)

			// Draining and already quiescent (no in-flight transactions):
			// completePendingDrain acks the Drain. Control commands are
			// serialized with no preemption, so a Reset queued behind the
			// drain is serviced only after the Drain acks.
			rob.State.ControlState = controlStateDraining
			rob.State.CurrentCmdID = 999
			rob.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
			rob.State.Transactions = nil

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsps []messaging.Msg
			for range 16 {
				modelingtest.Tick(rob)
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
			require.Equal(t, controlStateEnabled, rob.State.ControlState)
		})
	})
}
