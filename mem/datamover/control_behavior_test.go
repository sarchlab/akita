package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests: it drives the data mover
// one tick at a time and answers its Inside/Outside memory traffic by hand
// so the universal verbs can be observed deterministically (Drain only acks
// once the in-flight move finishes, Pause freezes intake, Reset wipes the
// move from any state). The protocol surface is covered separately in
// control_contract_test.go.
func TestDataMoverControlBehavior(t *testing.T) {
	var (
		engine       timing.Engine
		sim          *testsim.Simulation
		dataMover    *Comp
		topPort      messaging.Port
		ctrlPort     messaging.Port
		insidePort   messaging.Port
		outsidePort  messaging.Port
		insideRemote messaging.RemotePort
	)

	build := func() {
		spec := DefaultSpec()
		spec.BufferSize = 2048
		spec.InsideByteGranularity = 64
		spec.OutsideByteGranularity = 64

		insideRemote = messaging.RemotePort("InsideMem")

		spec.Inside.Destinations = []messaging.RemotePort{insideRemote}

		spec.Outside.Destinations = []messaging.RemotePort{messaging.RemotePort("OutsideMem")}
		dataMover = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("DataMover")
		dataMoverPorts := makePorts(sim, "DataMover", 16, 64, 64, 1024)
		dataMover.BindPort("Top", dataMoverPorts.Top)
		dataMover.BindPort("Inside", dataMoverPorts.Inside)
		dataMover.BindPort("Outside", dataMoverPorts.Outside)
		dataMover.BindPort("Control", dataMoverPorts.Control)

		topPort = dataMover.Ports.Top
		insidePort = dataMover.Ports.Inside
		outsidePort = dataMover.Ports.Outside
		ctrlPort = dataMover.Ports.Control
		for _, p := range []messaging.Port{
			topPort, ctrlPort, insidePort, outsidePort,
		} {
			(&ccNoopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	// makeMove builds a 64-byte outside->inside transfer, the minimal move
	// (one read on Outside, one write on Inside).
	makeMove := func() messaging.Msg {
		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   64},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),
		}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Cmd"),
			Dst: ctrlPort.AsRemote(),
		}

		return req
	}

	answerRead := func(port messaging.Port, read messaging.Msg) {
		data := make([]byte, int(read.Payload.(memprotocol.ReadReq).AccessByteSize))
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: data},
			ID:    sim.NewID(),
			Src:   read.Dst,
			Dst:   port.AsRemote(),
			RspTo: read.ID,
		}

		port.Deliver(rsp)
	}

	answerWrite := func(port messaging.Port, write messaging.Msg) {
		rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:    sim.NewID(),
			Src:   write.Dst,
			Dst:   port.AsRemote(),
			RspTo: write.ID,
		}

		port.Deliver(rsp)
	}

	// startMove delivers a move and ticks until it is active and the first
	// Outside read has been issued, returning that read.
	startMove := func() (messaging.Msg, bool) {
		topPort.Deliver(makeMove())

		var read messaging.Msg
		gotRead := false
		for i := 0; i < 64 && !gotRead; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := outsidePort.RetrieveOutgoing(); ok {
				_, gotRead = out.Payload.(memprotocol.ReadReq)
				read = out
			}
		}
		return read, gotRead
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build()
	}

	t.Run("acks Drain only after the in-flight move completes", func(t *testing.T) {
		setup1(t)

		read, gotRead := startMove()
		require.True(t, gotRead)
		require.True(t, dataMover.State.CurrentTransaction.Active)

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// The move is stuck waiting for its read response, so Drain must
		// stay pending and emit no ack.
		for range 5 {
			modelingtest.Tick(dataMover)
			require.Equal(t, controlStateDraining, dataMover.State.ControlState)
			require.True(t, dataMover.State.CurrentTransaction.Active)
			_, present0 := ctrlPort.RetrieveOutgoing()
			require.False(t, present0)
		}

		// Let the move finish: answer the read, then the write it triggers.
		answerRead(outsidePort, read)

		var drainRsp messaging.Msg
		gotDrainRsp := false
		var write messaging.Msg
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			modelingtest.Tick(dataMover)
			if !gotWrite {
				if out, ok := insidePort.RetrieveOutgoing(); ok {
					if _, ok := out.Payload.(memprotocol.WriteReq); ok {
						write = out

						gotWrite = true
						answerWrite(insidePort, write)
					}
				}
			}
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.Rsp); ok {
					moveDone = true
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					gotDrainRsp = true
				}
			}
		}

		require.True(t, gotWrite)
		require.True(t, gotDrainRsp)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		// The move completed (response emitted) before the async Drain ack.
		require.True(t, moveDone)
		require.False(t, dataMover.State.CurrentTransaction.Active)
		require.Equal(t, controlStatePaused, dataMover.State.ControlState)
	})

	t.Run("acks Drain only after outstanding destination writes are acked", func(t *testing.T) {
		setup1(t)

		read, gotRead := startMove()
		require.True(t, gotRead)

		// Answer the read so the data mover issues the destination write.
		answerRead(outsidePort, read)
		var write messaging.Msg
		gotWrite := false
		for i := 0; i < 64 && !gotWrite; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := insidePort.RetrieveOutgoing(); ok {
				_, gotWrite = out.Payload.(memprotocol.WriteReq)
				write = out
			}
		}
		require.True(t, gotWrite)
		require.True(t, dataMover.State.CurrentTransaction.Active)
		require.NotEmpty(t, dataMover.State.CurrentTransaction.PendingWrite)

		// Drain while the write's ack is still outstanding: the move is not
		// complete, so Drain must stay pending and emit no move response.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		for range 8 {
			modelingtest.Tick(dataMover)
			_, present1 := topPort.RetrieveOutgoing()
			require.False(t, present1)
			_, present2 := ctrlPort.RetrieveOutgoing()
			require.False(t, present2)
			require.True(t, dataMover.State.CurrentTransaction.Active)
		}

		// Ack the write; only now may the move finish and the Drain ack.
		answerWrite(insidePort, write)
		moveDone := false
		gotDrainRsp := false
		var drainRsp messaging.Msg
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.Rsp); ok {
					moveDone = true
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					gotDrainRsp = true
				}
			}
		}

		require.True(t, moveDone)
		require.True(t, gotDrainRsp)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		require.False(t, dataMover.State.CurrentTransaction.Active)
		require.Equal(t, controlStatePaused, dataMover.State.ControlState)
	})

	t.Run("drops a stale memory ack that arrives after Reset", func(t *testing.T) {
		setup1(t)

		// Move 1 issues an Outside read (readA), then is reset mid-flight.
		readA, ok := startMove()
		require.True(t, ok)

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					acked = true
				}
			}
		}
		require.True(t, acked)

		// Move 2 issues its own Outside read (readB) with a different ID.
		readB, ok := startMove()
		require.True(t, ok)
		require.NotEqual(t, readA.ID, readB.ID)

		// The stale response for readA arrives. It must be dropped (no panic),
		// leaving move 2 in flight.
		answerRead(outsidePort, readA)
		for range 8 {
			modelingtest.Tick(dataMover)
		}
		require.True(t, dataMover.State.CurrentTransaction.Active)

		// Move 2 completes once its own read and write are answered.
		answerRead(outsidePort, readB)
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !moveDone; i++ {
			modelingtest.Tick(dataMover)
			if !gotWrite {
				if out, ok := insidePort.RetrieveOutgoing(); ok {
					if _, ok := out.Payload.(memprotocol.WriteReq); ok {
						gotWrite = true
						answerWrite(insidePort, out)
					}
				}
			}
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.Rsp); ok {
					moveDone = true
				}
			}
		}
		require.True(t, moveDone)
	})

	t.Run("freezes incoming move requests while paused", func(t *testing.T) {
		setup1(t)

		dataMover.State.ControlState = controlStatePaused
		topPort.Deliver(makeMove())

		for range 5 {
			modelingtest.Tick(dataMover)
		}

		_, present3 := topPort.PeekIncoming()
		require.True(t, present3)
		require.False(t, dataMover.State.CurrentTransaction.Active)
		_, present4 := outsidePort.RetrieveOutgoing()
		require.False(t, present4)
	})

	t.Run("Reset wipes the in-flight move from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			_, gotRead := startMove()
			require.True(t, gotRead)
			require.True(t, dataMover.State.CurrentTransaction.Active)

			dataMover.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				modelingtest.Tick(dataMover)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, gotRsp)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			require.False(t, dataMover.State.CurrentTransaction.Active)
			require.Empty(t, dataMover.State.CurrentTransaction.PendingRead)
			require.Empty(t, dataMover.State.CurrentTransaction.PendingWrite)
			require.Equal(t, controlStateEnabled, dataMover.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent (no active transfer):
		// completePendingDrain acks the Drain. Control commands are serialized
		// with no preemption, so a Reset queued behind the drain is serviced
		// only after the Drain acks.
		dataMover.State.ControlState = controlStateDraining
		dataMover.State.CurrentCmdID = 999
		dataMover.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(dataMover)
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
		require.Equal(t, controlStateEnabled, dataMover.State.ControlState)
	})
}
