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

	build := func(limit int, window uint64) {
		spec := DefaultSpec()
		spec.BufferSize = window
		spec.MaxTransactions = limit
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
		build(1, 2048)
	}

	t.Run("shares read credits and drains a direction before reversing", func(t *testing.T) {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build(3, 64) // room for three moves, but just one read
		first, second, reverse, last := makeMove(), makeMove(), makeMove(), makeMove()
		reverse.Payload = datamoverprotocol.Req{SrcSide: "inside", DstSide: "outside", ByteSize: 64}
		for _, move := range []messaging.Msg{first, second, reverse, last} {
			topPort.Deliver(move)
		}

		tick := func() { modelingtest.Tick(dataMover) }
		tick()
		read1, ok := outsidePort.RetrieveOutgoing()
		require.True(t, ok)
		tick()
		tick()
		require.Len(t, dataMover.State.Transactions, 2)
		_, ok = outsidePort.RetrieveOutgoing()
		require.False(t, ok)
		head, ok := topPort.PeekIncoming()
		require.True(t, ok)
		require.Equal(t, reverse.ID, head.ID)

		answerRead(outsidePort, read1)
		tick()
		tick()
		write1, ok := insidePort.RetrieveOutgoing()
		require.True(t, ok)
		read2, ok := outsidePort.RetrieveOutgoing()
		require.True(t, ok)
		answerRead(outsidePort, read2)
		tick()
		tick()
		write2, ok := insidePort.RetrieveOutgoing()
		require.True(t, ok)
		answerWrite(insidePort, write2)
		tick()
		tick()
		_, ok = topPort.RetrieveOutgoing()
		require.False(t, ok, "later write completed first")

		answerWrite(insidePort, write1)
		tick()
		tick()
		tick()
		for _, want := range []uint64{first.ID, second.ID} {
			rsp, ok := topPort.RetrieveOutgoing()
			require.True(t, ok)
			require.Equal(t, want, rsp.RspTo)
		}
		read3, ok := insidePort.RetrieveOutgoing()
		require.True(t, ok)
		require.IsType(t, memprotocol.ReadReq{}, read3.Payload)
		head, ok = topPort.PeekIncoming()
		require.True(t, ok)
		require.Equal(t, last.ID, head.ID)
	})

	t.Run("last source chunk may start within the shared window", func(t *testing.T) {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build(3, 65)
		for range 3 {
			topPort.Deliver(makeMove())
		}
		for range 4 {
			modelingtest.Tick(dataMover)
		}
		require.Len(t, dataMover.State.Transactions, 3)
		for range 2 {
			read, ok := outsidePort.RetrieveOutgoing()
			require.True(t, ok)
			require.Equal(t, uint64(64), read.Payload.(memprotocol.ReadReq).AccessByteSize)
		}
		_, ok := outsidePort.RetrieveOutgoing()
		require.False(t, ok)
	})

	t.Run("acks Drain only after the in-flight move completes", func(t *testing.T) {
		setup1(t)

		read, gotRead := startMove()
		require.True(t, gotRead)
		require.NotEmpty(t, dataMover.State.Transactions)

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// The move is stuck waiting for its read response, so Drain must
		// stay pending and emit no ack.
		for range 5 {
			modelingtest.Tick(dataMover)
			require.Equal(t, controlStateDraining, dataMover.State.ControlState)
			require.NotEmpty(t, dataMover.State.Transactions)
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
		require.Empty(t, dataMover.State.Transactions)
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
		require.NotEmpty(t, dataMover.State.Transactions)
		require.NotEmpty(t, dataMover.State.Transactions[0].PendingWrite)

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
			require.NotEmpty(t, dataMover.State.Transactions)
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
		require.Empty(t, dataMover.State.Transactions)
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
		require.NotEmpty(t, dataMover.State.Transactions)

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
		require.Empty(t, dataMover.State.Transactions)
		_, present4 := outsidePort.RetrieveOutgoing()
		require.False(t, present4)
	})

	t.Run("Reset wipes the in-flight move from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			_, gotRead := startMove()
			require.True(t, gotRead)
			require.NotEmpty(t, dataMover.State.Transactions)

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
			require.Empty(t, dataMover.State.Transactions)
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
