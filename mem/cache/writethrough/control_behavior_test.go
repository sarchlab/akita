package writethrough

import (
	"bytes"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/cache/internal/maintenance"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The writethrough is downstream-dependent: a read miss issues a fetch
// request out the "Bottom" port and only retires once a matching
// memprotocol.DataReadyRsp (RspTo == the outgoing bottom read ID) is fed back in.
// The Drain test exploits exactly that: it proves the cache holds the Drain
// ack until those bottom responses arrive and every transaction completes.
func TestWritethroughCacheControlBehavior(t *testing.T) {
	const blockSize = uint64(64) // Log2BlockSize == 6

	var (
		engine     timing.Engine
		sim        *testsim.Simulation
		storage    *mem.Storage
		comp       *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port
	)

	dirLatency := 1
	build := func() {
		spec := DefaultSpec()
		spec.NumReqPerCycle = 1

		spec.DirectoryWidth = 1
		spec.BankWidth = 1
		spec.NumBanks = 1
		spec.NumMSHREntries = 8
		spec.NumWays = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = dirLatency
		spec.NumSets = 512
		spec.MaxInFlightRequests = 16

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("LowerCache")}
		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("L1Cache")
		compPorts := makePorts(sim, "L1Cache", 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		ctrlPort = comp.Ports.Control
		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			(&ccNoopConn{}).BindPort(p)
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
		}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Ctrl"),
			Dst: ctrlPort.AsRemote(),
		}

		return req
	}

	// makeFill builds the bottom DataReadyRsp that satisfies a captured
	// outgoing bottom read. RspTo matches the bottom read's ID (which the
	// cache stored as ReadToBottomMeta.ID), and Data is a full cache line so
	// the fetcher's slice [offset:offset+AccessByteSize] is always in range.
	makeFill := func(bottomRead messaging.Msg) messaging.Msg {
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: make([]byte, blockSize)},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("LowerCache"),
			Dst:          bottomPort.AsRemote(),
			RspTo:        bottomRead.ID,
			TrafficBytes: int(blockSize) + 4,
		}

		return rsp
	}

	// inflightCount counts transactions that are still being processed (not
	// yet removed by the respond stage).
	inflightCount := func() int {
		count := 0
		for i := range comp.State.Transactions {
			if !comp.State.Transactions[i].Removed {
				count++
			}
		}
		return count
	}

	// seedBlock writes a single valid (clean) directory block holding the
	// cache line that contains addr for pid. It returns the (setID, wayID)
	// it occupied so a test can assert directly against that block. The
	// writethrough directory only ever holds clean lines, so IsDirty stays
	// false.
	seedBlock := func(addr uint64, pid vm.PID) (int, int) {
		ds := &comp.State.DirectoryState
		cacheLineID := addr / blockSize * blockSize
		setID, _, _ := cache.DirectoryLookup(
			ds, comp.Spec.NumSets, int(blockSize), pid, cacheLineID)
		wayID := 0
		block := &ds.Sets[setID].Blocks[wayID]
		block.IsValid = true
		block.IsDirty = false
		block.Tag = cacheLineID
		block.PID = uint32(pid)
		return setID, wayID
	}

	// captureBottomReads drains every outgoing bottom read the cache has
	// issued so far, returning them so the test can fabricate matching fills.
	captureBottomReads := func() []messaging.Msg {
		reads := []messaging.Msg{}
		for {
			out, ok := bottomPort.RetrieveOutgoing()
			if !ok {
				break
			}
			if _, ok := out.Payload.(memprotocol.ReadReq); ok {
				reads = append(reads, out)
			}
		}
		return reads
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		storage = mem.NewStorage(4 * mem.GB)
		build()
	}

	t.Run("running maintenance uses configured directory latency and width", func(t *testing.T) {
		t.Cleanup(func() { dirLatency = 1 })
		ticksToACK := func(latency, lines int) int {
			dirLatency = latency
			setup1(t)
			addrs := make([]uint64, lines)
			for i := range addrs {
				addrs[i] = uint64(i) * blockSize
			}
			req := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
			req.Payload = memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, Addresses: addrs}
			ctrlPort.Deliver(req)
			for i := 1; i < 100; i++ {
				modelingtest.Tick(comp)
				if ack, ok := ctrlPort.RetrieveOutgoing(); ok {
					require.Equal(t, req.ID, ack.RspTo)
					return i
				}
			}
			t.Fatal("timed out waiting for Invalidate ACK")
			return 0
		}
		fast := ticksToACK(1, 1)
		slow := ticksToACK(5, 1)
		require.GreaterOrEqual(t, slow-fast, 4)
		require.Greater(t, ticksToACK(1, 17), fast)
	})

	t.Run("running invalidate orders a pending fill and a later same-line read", func(t *testing.T) {
		setup1(t)
		const addr = uint64(0x1000)
		older := makeRead(addr)
		topPort.Deliver(older)
		for range 32 {
			modelingtest.Tick(comp)
		}
		fetch, ok := bottomPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, addr, fetch.Payload.(memprotocol.ReadReq).Address)
		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		inv.Payload = memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, Addresses: []uint64{addr + 4, addr}}
		ctrlPort.Deliver(inv)
		later := makeRead(addr)
		topPort.Deliver(later)
		topPort.Deliver(makeRead(addr + 64))
		for range 32 {
			modelingtest.Tick(comp)
		}
		unrelated, ok := bottomPort.RetrieveOutgoing()
		require.True(t, ok, "unrelated miss behind the later read must issue while the target is held")
		require.Equal(t, addr+64, unrelated.Payload.(memprotocol.ReadReq).Address)
		_, ok = ctrlPort.PeekOutgoing()
		require.False(t, ok, "invalidate overtook the original fill")
		require.Len(t, comp.State.Maintenance.Deferred, 1)
		var saved bytes.Buffer
		require.NoError(t, comp.SaveCheckpoint(&saved))
		comp.State.Maintenance.Deferred = nil
		comp.State.Maintenance.ID = 0
		require.NoError(t, comp.LoadCheckpoint(&saved))
		bottomPort.Deliver(makeFill(fetch))
		for range 32 {
			modelingtest.Tick(comp)
		}
		reply, ok := topPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, older.ID, reply.RspTo)
		ack, ok := ctrlPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, inv.ID, ack.RspTo)
		require.True(t, ack.Payload.(memcontrolprotocol.Rsp).Success)
		refetch, ok := bottomPort.RetrieveOutgoing()
		require.True(t, ok, "later read must miss after invalidation")
		require.Equal(t, addr, refetch.Payload.(memprotocol.ReadReq).Address)
		_, ok = topPort.PeekOutgoing()
		require.False(t, ok, "later read consumed stale data")
	})

	t.Run("running invalidate waits for the older writethrough store ACK", func(t *testing.T) {
		setup1(t)
		const addr = uint64(0x1000)
		set, way := seedBlock(addr, 0)
		store := makeRead(addr)
		store.Payload = memprotocol.WriteReq{Address: addr, Data: []byte{0xaa}}
		topPort.Deliver(store)
		for range 32 {
			modelingtest.Tick(comp)
		}
		write, ok := bottomPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, addr, write.Payload.(memprotocol.WriteReq).Address)

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		inv.Payload = memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, Addresses: []uint64{addr}}
		ctrlPort.Deliver(inv)
		for range 32 {
			modelingtest.Tick(comp)
		}
		_, ok = ctrlPort.PeekOutgoing()
		require.False(t, ok)
		require.True(t, comp.State.DirectoryState.Sets[set].Blocks[way].IsValid)

		bottomPort.Deliver(messaging.Msg{ID: sim.NewID(), Src: "LowerCache", Dst: bottomPort.AsRemote(),
			RspTo: write.ID, Payload: memprotocol.WriteDoneRsp{}})
		for range 32 {
			modelingtest.Tick(comp)
		}
		done, ok := topPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, store.ID, done.RspTo)
		ack, ok := ctrlPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, inv.ID, ack.RspTo)
		require.False(t, comp.State.DirectoryState.Sets[set].Blocks[way].IsValid)
	})

	t.Run("drains all in-flight read misses before acking Drain", func(t *testing.T) {
		setup1(t)

		const n = 3

		// Deliver n read misses to distinct cache lines.
		for i := range n {
			topPort.Deliver(makeRead(uint64(i) * blockSize))
		}

		// Tick until the cache has issued all n bottom fetches and there are
		// in-flight transactions waiting on them.
		bottomReads := []messaging.Msg{}
		for i := 0; i < 256 && len(bottomReads) < n; i++ {
			modelingtest.Tick(comp)
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		require.Len(t, bottomReads, n)

		// Teeth: at least one transaction is genuinely in flight, waiting on
		// a bottom response.
		require.Greater(t, inflightCount(), 0)

		// Issue Drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// Tick a window WITHOUT feeding any bottom responses. Drain must wait
		// because transactions are still in flight.
		for range 16 {
			modelingtest.Tick(comp)
			// No completion can occur, so no DataReadyRsp should leave Top.
			_, present0 := topPort.RetrieveOutgoing()
			require.False(t, present0)
			// And no Drain ack yet on Control.
			_, present1 := ctrlPort.RetrieveOutgoing()
			require.False(t, present1)
		}
		require.True(t, comp.State.IsDraining)
		require.Greater(t, inflightCount(), 0)

		// Now feed the matching fill for every captured bottom read.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}

		// Tick until the Drain ack appears, counting completed reads on Top.
		completed := 0
		var drainRsp messaging.Msg
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			modelingtest.Tick(comp)

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
		// Every read miss finished by the time the async Drain ack is sent.
		require.Equal(t, n, completed)
		for i := range comp.State.Transactions {
			require.True(t, comp.State.Transactions[i].Removed)
		}
		// Drain ends Paused.
		require.False(t, comp.State.IsDraining)
		require.True(t, comp.State.IsPaused)
	})

	t.Run("does not abort an in-flight Drain when a Pause arrives", func(t *testing.T) {
		setup1(t)

		// Get a read miss in flight and capture its bottom fetch.
		topPort.Deliver(makeRead(0))
		bottomReads := []messaging.Msg{}
		for i := 0; i < 256 && len(bottomReads) == 0; i++ {
			modelingtest.Tick(comp)
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		require.Len(t, bottomReads, 1)
		require.Greater(t, inflightCount(), 0)

		// Begin draining, then a Pause arrives mid-drain. Control commands are
		// serialized, so the Pause stays queued until the drain finishes; it
		// must not freeze the pipeline (which only runs while !IsPaused) and
		// strand the drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		modelingtest.Tick(comp)
		require.True(t, comp.State.IsDraining)

		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)
		modelingtest.Tick(comp)

		// Feed the fill; the in-flight read retires and the drain still acks.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}
		drainAcked, pauseAcked := false, false
		for i := 0; i < 4096 && !drainAcked; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := ctrlPort.RetrieveOutgoing()
				if !ok {
					break
				}
				_, ok = out.Payload.(memcontrolprotocol.Rsp)
				r := out
				if !ok {
					continue
				}
				switch r.Payload.(memcontrolprotocol.Rsp).Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		require.True(t, pauseAcked)
		require.True(t, drainAcked)
		require.False(t, comp.State.IsDraining)
		require.True(t, comp.State.IsPaused)
	})

	t.Run("completes a Drain issued while paused with work in flight", func(t *testing.T) {
		setup1(t)

		// Get a read miss in flight, then capture its bottom fetch.
		topPort.Deliver(makeRead(0))
		bottomReads := []messaging.Msg{}
		for i := 0; i < 256 && len(bottomReads) == 0; i++ {
			modelingtest.Tick(comp)
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		require.Len(t, bottomReads, 1)
		require.Greater(t, inflightCount(), 0)

		// Pause: the in-flight transaction freezes (pipeline stops).
		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)
		pausedAck := false
		for i := 0; i < 64 && !pausedAck; i++ {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok &&
					out.
						RspTo == pause.ID {
					require.True(t, rsp.Success)
					pausedAck = true
				}
			}
		}
		require.True(t, pausedAck)
		require.True(t, comp.State.IsPaused)
		require.Greater(t, inflightCount(), 0)

		// Make the fill available, then Drain from the paused state. The drain
		// must clear the pause so the pipeline resumes, retire the frozen
		// transaction, and ack. Before the fix the pipeline stayed frozen
		// (IsPaused never cleared) and this Drain hung forever.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		var drainRsp messaging.Msg
		found := false
		for i := 0; i < 4096 && !found; i++ {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					found = true
				}
			}
		}

		require.True(t, found)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		for i := range comp.State.Transactions {
			require.True(t, comp.State.Transactions[i].Removed)
		}
		require.False(t, comp.State.IsDraining)
		require.True(t, comp.State.IsPaused)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		comp.State.IsPaused = true
		topPort.Deliver(makeRead(0))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into work, and nothing
		// is forwarded out the Bottom port, while paused.
		_, present2 := topPort.PeekIncoming()
		require.True(t, present2)
		require.Equal(t, 0, inflightCount())
		_, present3 := bottomPort.RetrieveOutgoing()
		require.False(t, present3)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, setStart func()) {
			t.Helper()

			setup1(t)

			// Get a read miss in flight.
			topPort.Deliver(makeRead(0))
			for i := 0; i < 256 && inflightCount() == 0; i++ {
				modelingtest.Tick(comp)
			}
			require.Greater(t, inflightCount(), 0)

			setStart()

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(comp)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, found)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			require.Empty(t, comp.State.Transactions)
			require.False(t, comp.State.IsPaused)
			require.False(t, comp.State.IsDraining)
		}
		t.Run("from Running", func(t *testing.T) {
			runCase(t, func() {
				comp.State.IsPaused = false
				comp.State.IsDraining = false
			})
		})
		t.Run("from Paused", func(t *testing.T) {
			runCase(t, func() {
				comp.State.IsPaused = true
			})
		})
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent (all transactions retired):
		// completePendingDrain acks the Drain. Control commands are serialized
		// with no preemption, so a Reset queued behind the drain is serviced
		// only after the Drain acks.
		comp.State.IsDraining = true
		comp.State.IsPaused = false
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(comp)
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
		require.False(t, comp.State.IsDraining)
		require.False(t, comp.State.IsPaused)
	})

	// driveCtrl delivers a control req and ticks until its ControlRsp comes
	// back (or the budget runs out), returning the matching Rsp and whether
	// it was found.
	driveCtrl := func(req messaging.Msg) (messaging.Msg, bool) {
		ctrlPort.Deliver(req)

		for range 64 {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(memcontrolprotocol.Rsp); ok &&
					out.
						RspTo == req.ID {
					return out,

						true
				}
			}
		}
		return messaging.Msg{Payload: memcontrolprotocol.Rsp{}}, false
	}

	t.Run("invalidates only directory blocks matching the address filter", func(t *testing.T) {
		setup1(t)

		// Two distinct, clean, resident cache lines.
		const addrA = uint64(0)
		const addrB = blockSize
		setA, wayA := seedBlock(addrA, vm.PID(1))
		setB, wayB := seedBlock(addrB, vm.PID(1))

		// They must occupy distinct blocks for the filter to be meaningful.
		require.NotEqual(t, [2]int{setB, wayB}, [2]int{setA, wayA})

		// Exercise the paused invalidation path.
		comp.State.IsPaused = true

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		payload1 := inv.Payload.(memcontrolprotocol.Req)
		payload1.Addresses = []uint64{addrA}
		inv.Payload = payload1
		payload2 := inv.Payload.(memcontrolprotocol.Req)
		payload2.PID = vm.PID(1)
		inv.Payload = payload2

		rsp, found := driveCtrl(inv)

		require.True(t, found)
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Empty(t, rsp.Payload.(memcontrolprotocol.Rsp).Error)

		// Only the filtered block A is dropped; block B survives untouched.
		blockA := comp.State.DirectoryState.Sets[setA].Blocks[wayA]
		blockB := comp.State.DirectoryState.Sets[setB].Blocks[wayB]
		require.False(t, blockA.IsValid)
		require.True(t, blockB.IsValid)
		require.Equal(t, addrB/blockSize*blockSize, blockB.Tag)
	})

	t.Run("acks Flush while paused without dropping any blocks", func(t *testing.T) {
		setup1(t)

		const addr = uint64(0)
		setID, wayID := seedBlock(addr, vm.PID(2))

		comp.State.IsPaused = true

		rsp, found := driveCtrl(makeCtrlReq(memcontrolprotocol.CmdFlush))

		require.True(t, found)
		require.Equal(t, memcontrolprotocol.CmdFlush, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Empty(t, rsp.Payload.(memcontrolprotocol.Rsp).Error)

		// Flush is a no-op for writethrough: the clean block is untouched.
		block := comp.State.DirectoryState.Sets[setID].Blocks[wayID]
		require.True(t, block.IsValid)
		require.Equal(t, addr/blockSize*blockSize, block.Tag)
	})

	t.Run("rejects unfiltered Invalidate issued while Enabled", func(t *testing.T) {
		setup1(t)

		setID, wayID := seedBlock(0, vm.PID(1))

		// Component is freshly built: Enabled (not paused, not draining).
		require.False(t, comp.State.IsPaused)
		require.False(t, comp.State.IsDraining)

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)

		rsp, found := driveCtrl(inv)

		require.True(t, found)
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, memcontrolprotocol.ErrMustBePausedOrDrained, rsp.Payload.(memcontrolprotocol.Rsp).Error)

		// Rejected Invalidate must leave the directory intact.
		block := comp.State.DirectoryState.Sets[setID].Blocks[wayID]
		require.True(t, block.IsValid)
	})

	makeRangeReq := func(
		cmd memcontrolprotocol.Command, pid vm.PID, start, end uint64,
	) messaging.Msg {
		req := makeCtrlReq(cmd)
		req.Payload = memcontrolprotocol.Req{Command: cmd, PID: pid, HasRange: true,
			Range: memcontrolprotocol.AddressRange{Start: start, End: end}}
		return req
	}

	// driveRange is driveCtrl with the tick budget of a lookup per set.
	driveRange := func(req messaging.Msg) (messaging.Msg, bool) {
		ctrlPort.Deliver(req)
		for range 4096 {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok && out.RspTo == req.ID {
				return out, true
			}
		}
		return messaging.Msg{}, false
	}

	// seedLines seeds lines of pid 1 in distinct sets and returns a lookup
	// of whether a line is still resident.
	seedLines := func(t *testing.T, addrs ...uint64) func(addr uint64) bool {
		t.Helper()
		sets := map[uint64]int{}
		used := map[int]bool{}
		for _, addr := range addrs {
			sets[addr], _ = seedBlock(addr, vm.PID(1))
			require.False(t, used[sets[addr]], "test lines must use distinct sets")
			used[sets[addr]] = true
		}
		return func(addr uint64) bool {
			return comp.State.DirectoryState.Sets[sets[addr]].Blocks[0].IsValid
		}
	}

	t.Run("a range Invalidate drops every line it intersects, with unaligned endpoints", func(t *testing.T) {
		const top = ^uint64(0) &^ (blockSize - 1)
		lines := []uint64{0, 0x1000, 0x1040, 0x1080, 0x10c0, top}
		cases := []struct {
			name       string
			pid        vm.PID
			start, end uint64
			want       []uint64
		}{
			{"unaligned inside two lines", 0, 0x1041, 0x1081, []uint64{0x1040, 0x1080}},
			{"end on a line boundary excludes that line", 1, 0x1040, 0x1080, []uint64{0x1040}},
			{"one byte", 1, 0x10ff, 0x1100, []uint64{0x10c0}},
			{"another PID", 2, 0, 0x2000, nil},
			{"nearly the whole address space", 0, 1, ^uint64(0), lines},
			{"from the middle to the top", 1, 0x1081, ^uint64(0), []uint64{0x1080, 0x10c0, top}},
		}
		for _, tc := range cases {
			for _, paused := range []bool{true, false} {
				setup1(t)
				resident := seedLines(t, lines...)
				comp.State.IsPaused = paused
				rsp, found := driveRange(
					makeRangeReq(memcontrolprotocol.CmdInvalidate, tc.pid, tc.start, tc.end))
				require.True(t, found, "%s (paused %v)", tc.name, paused)
				require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
				for _, line := range lines {
					require.Equal(t, !slices.Contains(tc.want, line), resident(line),
						"%s (paused %v): line %#x", tc.name, paused, line)
				}
				require.Zero(t, comp.State.Maintenance.Command)
				require.Equal(t, paused, comp.State.IsPaused)
			}
		}
	})

	t.Run("a running range Invalidate is bounded by the sets and survives a checkpoint", func(t *testing.T) {
		setup1(t)
		resident := seedLines(t, 0x1000, 1<<40)
		inv := makeRangeReq(memcontrolprotocol.CmdInvalidate, 0, 0, ^uint64(0))
		ctrlPort.Deliver(inv)
		modelingtest.Tick(comp)
		op := &comp.State.Maintenance
		require.True(t, op.HasRange)
		require.True(t, op.SweepSets)
		require.LessOrEqual(t, len(op.Lines)+op.LookupsInFlight, comp.Spec.NumSets,
			"the range must not be expanded into its lines")

		// A read of a line in the range waits for the ACK.
		read := makeRead(1 << 40)
		topPort.Deliver(read)
		for range 4 {
			modelingtest.Tick(comp)
		}
		require.Len(t, op.Deferred, 1)

		var saved bytes.Buffer
		require.NoError(t, comp.SaveCheckpoint(&saved))
		want := *op
		*op = maintenance.State{}
		require.NoError(t, comp.LoadCheckpoint(&saved))
		require.Equal(t, want, *op)

		var ack messaging.Msg
		for i := 0; i < 4096 && ack.ID == 0; i++ {
			modelingtest.Tick(comp)
			ack, _ = ctrlPort.RetrieveOutgoing()
		}
		require.Equal(t, inv.ID, ack.RspTo)
		require.True(t, ack.Payload.(memcontrolprotocol.Rsp).Success)
		require.False(t, resident(0x1000))
		for range 32 {
			modelingtest.Tick(comp)
		}
		fetches := captureBottomReads()
		require.Len(t, fetches, 1, "the held read must miss after the invalidation")
		require.Equal(t, uint64(1<<40), fetches[0].Payload.(memprotocol.ReadReq).Address)
	})

	t.Run("rejects invalid and ambiguous range addressing in every state", func(t *testing.T) {
		type AR = memcontrolprotocol.AddressRange
		cases := []struct {
			name string
			req  memcontrolprotocol.Req
			want string
		}{
			{"empty zero range", memcontrolprotocol.Req{HasRange: true}, memcontrolprotocol.ErrInvalidRange},
			{"empty range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 64, End: 64}},
				memcontrolprotocol.ErrInvalidRange},
			{"reversed range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 128, End: 64}},
				memcontrolprotocol.ErrInvalidRange},
			{"range without HasRange", memcontrolprotocol.Req{Range: AR{Start: 0, End: 64}},
				memcontrolprotocol.ErrInvalidRange},
			{"range and addresses", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0, End: 64},
				Addresses: []uint64{0}}, memcontrolprotocol.ErrAmbiguousAddressing},
		}
		for _, tc := range cases {
			for _, cmd := range []memcontrolprotocol.Command{
				memcontrolprotocol.CmdInvalidate, memcontrolprotocol.CmdFlush} {
				for _, paused := range []bool{true, false} {
					setup1(t)
					resident := seedLines(t, 0)
					comp.State.IsPaused = paused
					req := makeCtrlReq(cmd)
					payload := tc.req
					payload.Command = cmd
					req.Payload = payload
					rsp, found := driveCtrl(req)
					require.True(t, found, tc.name)
					require.Equal(t, req.ID, rsp.RspTo)
					require.Equal(t, memcontrolprotocol.Rsp{Command: cmd, Error: tc.want}, rsp.Payload, tc.name)
					require.True(t, resident(0))
					require.Zero(t, comp.State.Maintenance.Command)
				}
			}
		}
	})
}
