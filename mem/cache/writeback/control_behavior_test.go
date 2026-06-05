package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests for the writeback cache: it
// asserts the actual behavior the universal verbs promise (Drain quiescence,
// Pause freeze, Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go. The writeback cache is downstream-dependent: a read
// MISS issues a request out the Bottom port and only completes once the
// matching DataReadyRsp is fed back.
var _ = Describe("Write-Back Cache control behavior", func() {
	var (
		engine   timing.Engine
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		botPort  messaging.Port
		ctrlPort messaging.Port
	)

	const blockSize = 64 // 1 << Log2BlockSize(6)

	build := func() {
		spec := DefaultSpec()
		spec.TotalByteSize = 64 * 1024
		spec.NumBanks = 1
		spec.NumMSHREntry = 16
		spec.NumReqPerCycle = 4
		spec.WayAssociativity = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = 1
		spec.TopPortBufferSize = 16
		spec.BottomPortBufferSize = 16
		spec.ControlPortBufferSize = 16

		comp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				Storage: storage,
				AddressToPortMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("LowerCache"),
				},
			}).
			Build("L1Cache")

		topPort = comp.GetPortByName("Top")
		botPort = comp.GetPortByName("Bottom")
		ctrlPort = comp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, botPort, ctrlPort} {
			(&ccNoopConn{}).PlugIn(p)
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

	// makeFillRsp mirrors writeBufferStage.processDataReadyRsp's expectations:
	// the cache matches the response to the in-flight fetch by
	// FetchReadReqMeta.ID == msg.RspTo, so RspTo must equal the captured Bottom
	// read's ID. The data is block-sized.
	makeFillRsp := func(read mem.ReadReq) mem.DataReadyRsp {
		data := make([]byte, blockSize)
		for i := range data {
			data[i] = byte(i + 1)
		}
		rsp := mem.DataReadyRsp{Data: data}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = messaging.RemotePort("LowerCache")
		rsp.Dst = botPort.AsRemote()
		rsp.RspTo = read.ID
		rsp.TrafficClass = "mem.DataReadyRsp"
		return rsp
	}

	makeFilteredCtrlReq := func(
		cmd mem.ControlCommand,
		addresses []uint64,
	) mem.ControlReq {
		req := makeCtrlReq(cmd)
		req.Addresses = addresses
		return req
	}

	// residentDirtyBlock installs a valid+dirty block holding addr at way 0
	// of addr's set, writing distinct data into its backing storage slot so
	// a later flush write-back can be identified by its payload. It returns
	// the block's set ID so the test can inspect it after the verb.
	residentDirtyBlock := func(addr uint64, fill byte) int {
		setID := int(addr / uint64(blockSize) % uint64(comp.Spec().NumSets))
		block := &comp.State.DirectoryState.Sets[setID].Blocks[0]
		block.Tag = addr
		block.PID = 0
		block.IsValid = true
		block.IsDirty = true

		data := make([]byte, blockSize)
		for i := range data {
			data[i] = fill
		}
		Expect(storage.Write(block.CacheAddress, data)).To(Succeed())

		return setID
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(1 * mem.MB)
		build()
	})

	It("drains all in-flight read misses before acking Drain", func() {
		const n = 3

		// Deliver N distinct-block read misses.
		reads := make([]mem.ReadReq, n)
		for i := range n {
			reads[i] = makeRead(uint64(i) * blockSize)
			topPort.Deliver(reads[i])
		}

		// Tick until every miss has fired a Bottom read; capture them so we
		// can answer later. Until we answer, the cache cannot be quiescent.
		botReads := make([]mem.ReadReq, 0, n)
		for i := 0; i < 64 && len(botReads) < n; i++ {
			comp.Tick()
			for {
				out := botPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(mem.ReadReq); ok {
					botReads = append(botReads, r)
				}
			}
		}
		Expect(botReads).To(HaveLen(n))

		// Teeth: with misses in flight and nothing answered, the cache holds
		// live transactions and is NOT quiescent.
		Expect(comp.State.Transactions).ToNot(BeEmpty())
		Expect(cacheIsQuiescent(&comp.State)).To(BeFalse())

		// Issue Drain.
		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		// Tick a small window WITHOUT answering Bottom. Drain must wait: no
		// ControlRsp yet, state is Draining, still not quiescent.
		for range 5 {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok {
					Expect(rsp.Command).ToNot(Equal(mem.CmdDrain),
						"Drain must not ack before in-flight misses finish")
				}
			}
		}
		Expect(cacheState(comp.State.CacheState)).To(Equal(cacheStateDraining))
		Expect(cacheIsQuiescent(&comp.State)).To(BeFalse())

		// Now feed the matching fill response for each captured Bottom read.
		for _, br := range botReads {
			botPort.Deliver(makeFillRsp(br))
		}

		// Tick while counting completed Top responses and watching Control for
		// the async Drain ack.
		completed := 0
		var drainRsp mem.ControlRsp
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(mem.DataReadyRsp); ok {
					completed++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
					gotDrainRsp = true
				}
			}
		}

		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		Expect(completed).To(Equal(n))
		Expect(cacheIsQuiescent(&comp.State)).To(BeTrue())
		Expect(cacheState(comp.State.CacheState)).To(Equal(cacheStatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.CacheState = int(cacheStatePaused)
		topPort.Deliver(makeRead(0))

		for range 5 {
			comp.Tick()
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out Bottom, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(comp.State.Transactions).To(BeEmpty())
		Expect(botPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState cacheState) {
			// Get a transaction in flight via one read miss.
			topPort.Deliver(makeRead(0x10000))
			for i := 0; i < 64 && len(comp.State.Transactions) == 0; i++ {
				comp.Tick()
			}
			Expect(comp.State.Transactions).ToNot(BeEmpty())

			comp.State.CacheState = int(startState)

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp mem.ControlRsp
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, gotRsp = out.(mem.ControlRsp)
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.Transactions).To(BeEmpty())
			Expect(cacheState(comp.State.CacheState)).
				To(Equal(cacheStateRunning))
		},
		Entry("from Running", cacheStateRunning),
		Entry("from Paused", cacheStatePaused),
		Entry("from Draining", cacheStateDraining),
	)

	It("drops only the address-filtered block on Invalidate, keeping the "+
		"other resident block", func() {
		// Two resident blocks in different sets so they never collide.
		const addrKeep = uint64(0x0)
		const addrDrop = uint64(blockSize) // next block, set 1
		setKeep := residentDirtyBlock(addrKeep, 0xAA)
		setDrop := residentDirtyBlock(addrDrop, 0xBB)

		// Both are valid before the verb.
		Expect(comp.State.DirectoryState.Sets[setKeep].Blocks[0].IsValid).
			To(BeTrue())
		Expect(comp.State.DirectoryState.Sets[setDrop].Blocks[0].IsValid).
			To(BeTrue())

		// Pause: Invalidate is only legal once paused.
		comp.State.CacheState = int(cacheStatePaused)

		inv := makeFilteredCtrlReq(mem.CmdInvalidate, []uint64{addrDrop})
		ctrlPort.Deliver(inv)

		var rsp mem.ControlRsp
		gotRsp := false
		for i := 0; i < 64 && !gotRsp; i++ {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				rsp, gotRsp = out.(mem.ControlRsp)
			}
		}

		Expect(gotRsp).To(BeTrue())
		Expect(rsp.Command).To(Equal(mem.CmdInvalidate))
		Expect(rsp.Success).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(inv.ID))

		// Only the filtered block was dropped; the other stays resident.
		dropBlock := &comp.State.DirectoryState.Sets[setDrop].Blocks[0]
		keepBlock := &comp.State.DirectoryState.Sets[setKeep].Blocks[0]
		Expect(dropBlock.IsValid).To(BeFalse(),
			"filtered block must be invalidated")
		Expect(keepBlock.IsValid).To(BeTrue(),
			"unfiltered block must remain resident")
		Expect(keepBlock.Tag).To(Equal(addrKeep))

		// Invalidate discards dirty data silently: no write-back is emitted.
		Expect(botPort.RetrieveOutgoing()).To(BeNil())
	})

	It("rejects Invalidate while Enabled with ErrMustBePausedOrDrained",
		func() {
			residentDirtyBlock(0x0, 0xAA)
			// Cache is Running (Enabled) as freshly built.
			Expect(cacheState(comp.State.CacheState)).
				To(Equal(cacheStateRunning))

			inv := makeCtrlReq(mem.CmdInvalidate)
			ctrlPort.Deliver(inv)

			var rsp mem.ControlRsp
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, gotRsp = out.(mem.ControlRsp)
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdInvalidate))
			Expect(rsp.Success).To(BeFalse())
			Expect(rsp.Error).To(Equal(control.ErrMustBePausedOrDrained))

			// The block is untouched because the verb was rejected.
			Expect(comp.State.DirectoryState.Sets[0].Blocks[0].IsValid).
				To(BeTrue())
		})

	It("writes back only the address-filtered dirty block on Flush, "+
		"leaving the other dirty block in place", func() {
		const addrFlush = uint64(0x0)
		const addrKeep = uint64(blockSize) // set 1
		const flushFill = byte(0x11)
		const keepFill = byte(0x22)
		setFlush := residentDirtyBlock(addrFlush, flushFill)
		setKeep := residentDirtyBlock(addrKeep, keepFill)

		// Pause so Flush is legal.
		comp.State.CacheState = int(cacheStatePaused)

		flush := makeFilteredCtrlReq(mem.CmdFlush, []uint64{addrFlush})
		ctrlPort.Deliver(flush)

		// Drive to completion, capturing every Bottom write-back and the
		// async Flush ack. The lower memory must answer write-backs with a
		// WriteDoneRsp or the flush never finishes.
		botWrites := []mem.WriteReq{}
		var flushRsp mem.ControlRsp
		gotFlushRsp := false
		for i := 0; i < 4096 && !gotFlushRsp; i++ {
			comp.Tick()
			for {
				out := botPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if w, ok := out.(mem.WriteReq); ok {
					botWrites = append(botWrites, w)
					done := mem.WriteDoneRsp{}
					done.ID = timing.GetIDGenerator().Generate()
					done.Src = messaging.RemotePort("LowerCache")
					done.Dst = botPort.AsRemote()
					done.RspTo = w.ID
					done.TrafficClass = "mem.WriteDoneRsp"
					botPort.Deliver(done)
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if r, ok := out.(mem.ControlRsp); ok &&
					r.Command == mem.CmdFlush {
					flushRsp = r
					gotFlushRsp = true
				}
			}
		}

		Expect(gotFlushRsp).To(BeTrue())
		Expect(flushRsp.Success).To(BeTrue())
		Expect(flushRsp.RspTo).To(Equal(flush.ID))

		// Exactly one write-back, for the filtered block only.
		Expect(botWrites).To(HaveLen(1))
		Expect(botWrites[0].Address).To(Equal(addrFlush))
		Expect(botWrites[0].Data).To(HaveLen(blockSize))
		Expect(botWrites[0].Data[0]).To(Equal(flushFill),
			"the written-back payload must be the filtered block's data")
		for _, b := range botWrites[0].Data {
			Expect(b).To(Equal(flushFill))
		}

		// The flushed block is now clean but still valid; the unfiltered
		// dirty block was neither written back nor cleaned.
		flushBlock := &comp.State.DirectoryState.Sets[setFlush].Blocks[0]
		keepBlock := &comp.State.DirectoryState.Sets[setKeep].Blocks[0]
		Expect(flushBlock.IsValid).To(BeTrue())
		Expect(flushBlock.IsDirty).To(BeFalse(),
			"flushed block must be clean after write-back")
		Expect(keepBlock.IsValid).To(BeTrue())
		Expect(keepBlock.IsDirty).To(BeTrue(),
			"unfiltered dirty block must stay dirty")
	})
})
