package writeback

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests for the writeback cache: it
// asserts the actual behavior the universal verbs promise (Drain quiescence,
// Pause freeze, Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go. The writeback cache is downstream-dependent: a read
// MISS issues a request out the Bottom port and only completes once the
// matching DataReadyRsp is fed back.
func TestWriteBackCacheControlBehavior(t *testing.T) {
	var (
		engine   timing.Engine
		sim      *testsim.Simulation
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		botPort  messaging.Port
		ctrlPort messaging.Port
	)

	const blockSize = 64 // 1 << Log2BlockSize(6)

	dirLatency := 1
	requestsPerCycle := 4
	build := func() {
		spec := DefaultSpec()
		spec.NumSets = 512
		spec.NumBanks = 1
		spec.NumMSHREntries = 16
		spec.NumReqPerCycle = requestsPerCycle

		spec.DirectoryWidth = 4
		spec.BankWidth = 4
		spec.NumWays = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = dirLatency

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("LowerCache")}
		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("L1Cache")
		compPorts := makePorts(sim, "L1Cache", 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)
		plugNoopConn(comp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		topPort = comp.Ports.Top
		botPort = comp.Ports.Bottom
		ctrlPort = comp.Ports.Control
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

	// makeFillRsp mirrors writeBufferStage.processDataReadyRsp's expectations:
	// the cache matches the response to the in-flight fetch by
	// FetchReadReqMeta.ID == msg.RspTo, so RspTo must equal the captured Bottom
	// read's ID. The data is block-sized.
	makeFillRsp := func(read messaging.Msg) messaging.Msg {
		data := make([]byte, blockSize)
		for i := range data {
			data[i] = byte(i + 1)
		}
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: data},
			ID:    sim.NewID(),
			Src:   messaging.RemotePort("LowerCache"),
			Dst:   botPort.AsRemote(),
			RspTo: read.ID,
		}

		return rsp
	}

	makeFilteredCtrlReq := func(
		cmd memcontrolprotocol.Command,
		addresses []uint64,
	) messaging.Msg {
		req := makeCtrlReq(cmd)
		payload1 := req.Payload.(memcontrolprotocol.Req)
		payload1.Addresses = addresses
		req.Payload = payload1
		return req
	}

	// residentDirtyBlock installs a valid+dirty block holding addr at way 0
	// of addr's set, writing distinct data into its backing storage slot so
	// a later flush write-back can be identified by its payload. It returns
	// the block's set ID so the test can inspect it after the verb.
	residentDirtyBlock := func(addr uint64, fill byte) int {
		setID := cache.DirectorySetID(addr, blockSize, comp.Spec.NumSets)
		block := &comp.State.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = addr
		block.PID = 0
		block.IsValid = true
		block.IsDirty = true

		data := make([]byte, blockSize)
		for i := range data {
			data[i] = fill
		}
		storage.Write(block.CacheAddress, data)

		return setID
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		storage = mem.NewStorage(1 * mem.MB)
		build()
	}

	t.Run("running maintenance uses configured directory latency and width", func(t *testing.T) {
		requestsPerCycle = 1
		t.Cleanup(func() { requestsPerCycle, dirLatency = 4, 1 })
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
		fetch, ok := botPort.RetrieveOutgoing()
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
		unrelated, ok := botPort.RetrieveOutgoing()
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
		botPort.Deliver(makeFillRsp(fetch))
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
		refetch, ok := botPort.RetrieveOutgoing()
		require.True(t, ok, "later read must miss after invalidation")
		require.Equal(t, addr, refetch.Payload.(memprotocol.ReadReq).Address)
		_, ok = topPort.PeekOutgoing()
		require.False(t, ok, "later read consumed stale data")
	})

	t.Run("running flush orders stores around the lower write acknowledgement", func(t *testing.T) {
		setup1(t)
		const addr = uint64(0x1000)
		store := func(value byte) messaging.Msg {
			req := makeRead(addr)
			req.Payload = memprotocol.WriteReq{Address: addr, Data: []byte{value}}
			return req
		}
		first := store(7)
		topPort.Deliver(first)
		for range 32 {
			modelingtest.Tick(comp)
		}
		fill, ok := botPort.RetrieveOutgoing()
		require.True(t, ok)
		flush := makeFilteredCtrlReq(memcontrolprotocol.CmdFlush, []uint64{addr + 1})
		ctrlPort.Deliver(flush)
		later := store(9)
		topPort.Deliver(later)
		botPort.Deliver(makeFillRsp(fill))
		for range 32 {
			modelingtest.Tick(comp)
		}
		write, ok := botPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, byte(7), write.Payload.(memprotocol.WriteReq).Data[0])
		older, ok := topPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, first.ID, older.RspTo)
		_, ok = topPort.PeekOutgoing()
		require.False(t, ok)
		_, ok = ctrlPort.PeekOutgoing()
		require.False(t, ok)
		botPort.Deliver(messaging.Msg{ID: sim.NewID(), RspTo: write.ID, Payload: memprotocol.WriteDoneRsp{}})
		for range 32 {
			modelingtest.Tick(comp)
		}
		ack, ok := ctrlPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, flush.ID, ack.RspTo)
		require.True(t, ack.Payload.(memcontrolprotocol.Rsp).Success)
		last, ok := topPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, later.ID, last.RspTo)
		set, way, found := cache.DirectoryLookup(&comp.State.DirectoryState, comp.Spec.NumSets, blockSize, 0, addr)
		require.True(t, found)
		block := comp.State.DirectoryState.Sets[set].Blocks[way]
		require.True(t, block.IsDirty)
		require.Equal(t, []byte{9}, storage.Read(block.CacheAddress, 1))
	})

	t.Run("running wildcard invalidation visits distinct tags and holds new accesses until ACK", func(t *testing.T) {
		setup1(t)
		const a, b = uint64(64), uint64(128)
		for _, addr := range []uint64{a, b} {
			set := residentDirtyBlock(addr, 42)
			blocks := comp.State.DirectoryState.Sets[set].Blocks
			blocks[0].PID = 1
			blocks[1].Tag, blocks[1].PID, blocks[1].IsValid = addr, 2, true
		}
		cmd := makeFilteredCtrlReq(memcontrolprotocol.CmdInvalidate, []uint64{a + 1, b, a})
		for ctrlPort.CanSend() {
			ctrlPort.Send(makeCtrlRsp(comp, memcontrolprotocol.CmdEnable, "Ctrl", 0, true, ""))
		}
		ctrlPort.Deliver(cmd)
		topPort.Deliver(makeRead(a))
		modelingtest.Tick(comp)
		require.Len(t, comp.State.Maintenance.Tags, 2)
		for range 32 {
			modelingtest.Tick(comp)
		}
		for _, addr := range []uint64{a, b} {
			blocks := comp.State.DirectoryState.Sets[cache.DirectorySetID(addr, blockSize, comp.Spec.NumSets)].Blocks
			require.False(t, blocks[0].IsValid)
			require.False(t, blocks[1].IsValid)
		}
		require.Equal(t, cmd.ID, comp.State.Maintenance.ID)
		require.Len(t, comp.State.Maintenance.Deferred, 1)
		_, ok := botPort.PeekOutgoing()
		require.False(t, ok)
		for !ctrlPort.CanSend() {
			ctrlPort.RetrieveOutgoing()
		}
		for range 32 {
			modelingtest.Tick(comp)
		}
		var ack messaging.Msg
		for {
			out, ok := ctrlPort.RetrieveOutgoing()
			if !ok {
				break
			}
			if out.RspTo == cmd.ID {
				ack = out
			}
		}
		require.Equal(t, cmd.ID, ack.RspTo)
		read, ok := botPort.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, a, read.Payload.(memprotocol.ReadReq).Address)
	})

	t.Run("running flush of an evicted line waits only for that writeback", func(t *testing.T) {
		setup1(t)
		const victim = uint64(0)
		set := residentDirtyBlock(victim, 0x3a)
		addr := uint64(blockSize)
		for cache.DirectorySetID(addr, blockSize, comp.Spec.NumSets) != set {
			addr += blockSize
		}
		topPort.Deliver(makeRead(addr))
		for range 32 {
			modelingtest.Tick(comp)
		}
		var write messaging.Msg
		for {
			out, ok := botPort.RetrieveOutgoing()
			if !ok {
				break
			}
			if _, ok := out.Payload.(memprotocol.WriteReq); ok {
				write = out
			}
		}
		require.NotZero(t, write.ID)
		require.Equal(t, victim, write.Payload.(memprotocol.WriteReq).Address)
		ctrlPort.Deliver(makeFilteredCtrlReq(memcontrolprotocol.CmdFlush, []uint64{victim}))
		for range 32 {
			modelingtest.Tick(comp)
		}
		_, ok := ctrlPort.PeekOutgoing()
		require.False(t, ok, "victim data still in flight")
		botPort.Deliver(messaging.Msg{ID: sim.NewID(), RspTo: write.ID, Payload: memprotocol.WriteDoneRsp{}})
		for range 32 {
			modelingtest.Tick(comp)
		}
		ack, ok := ctrlPort.RetrieveOutgoing()
		require.True(t, ok, "unrelated replacement's unanswered read must not delay flush")
		require.True(t, ack.Payload.(memcontrolprotocol.Rsp).Success)
	})

	t.Run("drains all in-flight read misses before acking Drain", func(t *testing.T) {
		setup1(t)

		const n = 3

		// Deliver N distinct-block read misses.
		reads := make([]messaging.Msg, n)
		for i := range n {
			reads[i] = makeRead(uint64(i) * blockSize)
			topPort.Deliver(reads[i])
		}

		// Tick until every miss has fired a Bottom read; capture them so we
		// can answer later. Until we answer, the cache cannot be quiescent.
		botReads := make([]messaging.Msg, 0, n)
		for i := 0; i < 64 && len(botReads) < n; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := botPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memprotocol.ReadReq); ok {
					botReads = append(botReads, out)
				}
			}
		}
		require.Len(t, botReads, n)

		// Teeth: with misses in flight and nothing answered, the cache holds
		// live transactions and is NOT quiescent.
		require.NotEmpty(t, comp.State.Transactions)
		require.False(t, cacheIsQuiescent(&comp.State))

		// Issue Drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// Tick a small window WITHOUT answering Bottom. Drain must wait: no
		// ControlRsp yet, state is Draining, still not quiescent.
		for range 5 {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok {
					require.NotEqual(t, memcontrolprotocol.CmdDrain, rsp.Command, "Drain must not ack before in-flight misses finish")
				}
			}
		}
		require.Equal(t, cacheStateDraining, cacheState(comp.State.CacheState))
		require.False(t, cacheIsQuiescent(&comp.State))

		// Now feed the matching fill response for each captured Bottom read.
		for _, br := range botReads {
			botPort.Deliver(makeFillRsp(br))
		}

		// Tick while counting completed Top responses and watching Control for
		// the async Drain ack.
		completed := 0
		var drainRsp messaging.Msg
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
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

					gotDrainRsp = true
				}
			}
		}

		require.True(t, gotDrainRsp)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		require.Equal(t, n, completed)
		require.True(t, cacheIsQuiescent(&comp.State))
		require.Equal(t, cacheStatePaused, cacheState(comp.State.CacheState))
	})

	t.Run("leaves a Pause queued while a flush is in progress", func(t *testing.T) {
		setup1(t)

		// An active flush that is still pre-flushing (waiting on an in-flight
		// transaction) so it cannot finalize.
		comp.State.CacheState = int(cacheStatePreFlushing)
		comp.State.HasProcessingFlush = true
		comp.State.Transactions = []transactionState{{}}

		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)

		// Control commands are serialized: while the flush is in progress the
		// Pause is not dequeued, so it is never acked and the flush state is
		// preserved (aborting it would strand the flusher).
		for range 16 {
			modelingtest.Tick(comp)
			_, present0 := ctrlPort.RetrieveOutgoing()
			require.False(t, present0)
		}
		require.True(t, comp.State.HasProcessingFlush)
		require.Equal(t, cacheStatePreFlushing, cacheState(comp.State.CacheState))
		// The Pause is still waiting on the Control port, to be handled once the
		// flush settles.
		_, present1 := ctrlPort.PeekIncoming()
		require.True(t, present1)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		comp.State.CacheState = int(cacheStatePaused)
		topPort.Deliver(makeRead(0))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out Bottom, while paused.
		_, present2 := topPort.PeekIncoming()
		require.True(t, present2)
		require.Empty(t, comp.State.Transactions)
		_, present3 := botPort.RetrieveOutgoing()
		require.False(t, present3)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState cacheState) {
			t.Helper()

			setup1(t)

			// Get a transaction in flight via one read miss.
			topPort.Deliver(makeRead(0x10000))
			for i := 0; i < 64 && len(comp.State.Transactions) == 0; i++ {
				modelingtest.Tick(comp)
			}
			require.NotEmpty(t, comp.State.Transactions)

			comp.State.CacheState = int(startState)

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				modelingtest.Tick(comp)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, gotRsp)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			require.Empty(t, comp.State.Transactions)
			require.Equal(t, cacheStateRunning, cacheState(comp.State.CacheState))
		}
		t.Run("from Running", func(t *testing.T) { runCase(t, cacheStateRunning) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, cacheStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent: completePendingDrain acks the Drain.
		// Control commands are serialized with no preemption, so a Reset queued
		// behind the drain is serviced only after the Drain acks.
		comp.State.CacheState = int(cacheStateDraining)
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
		require.Equal(t, cacheStateRunning, cacheState(comp.State.CacheState))
	})

	t.Run("drops only the address-filtered block on Invalidate, keeping the "+
		"other resident block", func(t *testing.T) {
		setup1(t)

		// Two resident blocks in different sets so they never collide.
		const addrKeep = uint64(0x0)
		const addrDrop = uint64(blockSize) // next block, set 1
		setKeep := residentDirtyBlock(addrKeep, 0xAA)
		setDrop := residentDirtyBlock(addrDrop, 0xBB)

		// Both are valid before the verb.
		require.True(t, comp.State.DirectoryState.Sets[setKeep].Blocks[0].IsValid)
		require.True(t, comp.State.DirectoryState.Sets[setDrop].Blocks[0].IsValid)

		// Pause: Invalidate is only legal once paused.
		comp.State.CacheState = int(cacheStatePaused)

		inv := makeFilteredCtrlReq(memcontrolprotocol.CmdInvalidate, []uint64{addrDrop})
		ctrlPort.Deliver(inv)

		var rsp messaging.Msg
		gotRsp := false
		for i := 0; i < 64 && !gotRsp; i++ {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
				rsp = out
			}
		}

		require.True(t, gotRsp)
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, inv.ID, rsp.RspTo)

		// Only the filtered block was dropped; the other stays resident.
		dropBlock := &comp.State.DirectoryState.Sets[setDrop].Blocks[0]
		keepBlock := &comp.State.DirectoryState.Sets[setKeep].Blocks[0]
		require.False(t, dropBlock.IsValid, "filtered block must be invalidated")
		require.True(t, keepBlock.IsValid, "unfiltered block must remain resident")
		require.Equal(t, addrKeep, keepBlock.Tag)

		// Invalidate discards dirty data silently: no write-back is emitted.
		_, present4 := botPort.RetrieveOutgoing()
		require.False(t, present4)
	})

	t.Run("rejects Invalidate while Enabled with ErrMustBePausedOrDrained", func(t *testing.T) {
		setup1(t)

		residentDirtyBlock(0x0, 0xAA)
		// Cache is Running (Enabled) as freshly built.
		require.Equal(t, cacheStateRunning, cacheState(comp.State.CacheState))

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		ctrlPort.Deliver(inv)

		var rsp messaging.Msg
		gotRsp := false
		for i := 0; i < 64 && !gotRsp; i++ {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
				rsp = out
			}
		}

		require.True(t, gotRsp)
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, memcontrolprotocol.ErrMustBePausedOrDrained, rsp.Payload.(memcontrolprotocol.Rsp).Error)

		// The block is untouched because the verb was rejected.
		require.True(t, comp.State.DirectoryState.Sets[0].Blocks[0].IsValid)
	})

	t.Run("writes back only the address-filtered dirty block on Flush, "+
		"leaving the other dirty block in place", func(t *testing.T) {
		setup1(t)

		const addrFlush = uint64(0x0)
		const addrKeep = uint64(blockSize) // set 1
		const flushFill = byte(0x11)
		const keepFill = byte(0x22)
		setFlush := residentDirtyBlock(addrFlush, flushFill)
		setKeep := residentDirtyBlock(addrKeep, keepFill)

		// Pause so Flush is legal.
		comp.State.CacheState = int(cacheStatePaused)

		flush := makeFilteredCtrlReq(memcontrolprotocol.CmdFlush, []uint64{addrFlush})
		ctrlPort.Deliver(flush)

		// Drive to completion, capturing every Bottom write-back and the
		// async Flush ack. The lower memory must answer write-backs with a
		// WriteDoneRsp or the flush never finishes.
		botWrites := []messaging.Msg{}
		var flushRsp messaging.Msg
		gotFlushRsp := false
		for i := 0; i < 4096 && !gotFlushRsp; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := botPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memprotocol.WriteReq); ok {
					botWrites = append(botWrites, out)
					done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
						ID:  sim.NewID(),
						Src: messaging.RemotePort("LowerCache"),
						Dst: botPort.AsRemote(),
						RspTo: out.
							ID,
					}

					botPort.Deliver(done)
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdFlush {
					flushRsp = out

					gotFlushRsp = true
				}
			}
		}

		require.True(t, gotFlushRsp)
		require.True(t, flushRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, flush.ID, flushRsp.RspTo)

		// Exactly one write-back, for the filtered block only.
		require.Len(t, botWrites, 1)
		require.Equal(t, addrFlush, botWrites[0].Payload.(memprotocol.WriteReq).Address)
		require.Len(t, botWrites[0].Payload.(memprotocol.WriteReq).Data, blockSize)
		require.Equal(
			t,
			flushFill,
			botWrites[0].Payload.(memprotocol.WriteReq).Data[0],
			"the written-back payload must be the filtered block's data",
		)
		for _, b := range botWrites[0].Payload.(memprotocol.WriteReq).Data {
			require.Equal(t, flushFill, b)
		}

		// The flushed block is now clean but still valid; the unfiltered
		// dirty block was neither written back nor cleaned.
		flushBlock := &comp.State.DirectoryState.Sets[setFlush].Blocks[0]
		keepBlock := &comp.State.DirectoryState.Sets[setKeep].Blocks[0]
		require.True(t, flushBlock.IsValid)
		require.False(t, flushBlock.IsDirty, "flushed block must be clean after write-back")
		require.True(t, keepBlock.IsValid)
		require.True(t, keepBlock.IsDirty, "unfiltered dirty block must stay dirty")
	})
}
