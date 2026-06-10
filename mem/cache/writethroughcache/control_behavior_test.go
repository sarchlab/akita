package writethroughcache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The writethroughcache is downstream-dependent: a read miss issues a fetch
// request out the "Bottom" port and only retires once a matching
// memprotocol.DataReadyRsp (RspTo == the outgoing bottom read ID) is fed back in.
// The Drain test exploits exactly that: it proves the cache holds the Drain
// ack until those bottom responses arrive and every transaction completes.
var _ = Describe("Writethrough cache control behavior", func() {
	const blockSize = uint64(64) // Log2BlockSize == 6

	var (
		engine     timing.Engine
		storage    *mem.Storage
		comp       *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.NumReqPerCycle = 1
		spec.NumBanks = 1
		spec.NumMSHREntry = 8
		spec.WayAssociativity = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = 1
		spec.TotalByteSize = 64 * 1024
		spec.MaxNumConcurrentTrans = 16

		reg := modeling.NewStandaloneRegistrar(engine)
		comp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				Storage: storage,
				AddressMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("LowerCache"),
				},
			}).
			Build("L1Cache")

		// Build declares the ports; assign every declared port instance
		// (the caller now chooses the buffer sizes) before the component
		// is ticked.
		for _, name := range []string{"Top", "Bottom", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			comp.AssignPort(name, p)
		}

		topPort = comp.GetPortByName("Top")
		bottomPort = comp.GetPortByName("Bottom")
		ctrlPort = comp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			(&ccNoopConn{}).PlugIn(p)
		}
	}

	makeRead := func(addr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "memprotocol.ReadReq"
		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) memcontrolprotocol.Req {
		req := memcontrolprotocol.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		return req
	}

	// makeFill builds the bottom DataReadyRsp that satisfies a captured
	// outgoing bottom read. RspTo matches the bottom read's ID (which the
	// cache stored as ReadToBottomMeta.ID), and Data is a full cache line so
	// the fetcher's slice [offset:offset+AccessByteSize] is always in range.
	makeFill := func(bottomRead memprotocol.ReadReq) memprotocol.DataReadyRsp {
		rsp := memprotocol.DataReadyRsp{Data: make([]byte, blockSize)}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = messaging.RemotePort("LowerCache")
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = bottomRead.ID
		rsp.TrafficBytes = int(blockSize) + 4
		rsp.TrafficClass = "memprotocol.DataReadyRsp"
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
			ds, comp.Spec().NumSets, int(blockSize), pid, cacheLineID)
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
	captureBottomReads := func() []memprotocol.ReadReq {
		reads := []memprotocol.ReadReq{}
		for {
			out := bottomPort.RetrieveOutgoing()
			if out == nil {
				break
			}
			if r, ok := out.(memprotocol.ReadReq); ok {
				reads = append(reads, r)
			}
		}
		return reads
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(4 * mem.GB)
		build()
	})

	It("drains all in-flight read misses before acking Drain", func() {
		const n = 3

		// Deliver n read misses to distinct cache lines.
		for i := range n {
			topPort.Deliver(makeRead(uint64(i) * blockSize))
		}

		// Tick until the cache has issued all n bottom fetches and there are
		// in-flight transactions waiting on them.
		bottomReads := []memprotocol.ReadReq{}
		for i := 0; i < 256 && len(bottomReads) < n; i++ {
			comp.Tick()
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		Expect(bottomReads).To(HaveLen(n))

		// Teeth: at least one transaction is genuinely in flight, waiting on
		// a bottom response.
		Expect(inflightCount()).To(BeNumerically(">", 0))

		// Issue Drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// Tick a window WITHOUT feeding any bottom responses. Drain must wait
		// because transactions are still in flight.
		for range 16 {
			comp.Tick()
			// No completion can occur, so no DataReadyRsp should leave Top.
			Expect(topPort.RetrieveOutgoing()).To(BeNil())
			// And no Drain ack yet on Control.
			Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())
		}
		Expect(comp.State.IsDraining).To(BeTrue())
		Expect(inflightCount()).To(BeNumerically(">", 0))

		// Now feed the matching fill for every captured bottom read.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}

		// Tick until the Drain ack appears, counting completed reads on Top.
		completed := 0
		var drainRsp memcontrolprotocol.Rsp
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			comp.Tick()

			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(memprotocol.DataReadyRsp); ok {
					completed++
				}
			}

			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
					rsp.Command == memcontrolprotocol.CmdDrain {
					drainRsp = rsp
					drainFound = true
				}
			}
		}

		Expect(drainFound).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Every read miss finished by the time the async Drain ack is sent.
		Expect(completed).To(Equal(n))
		for i := range comp.State.Transactions {
			Expect(comp.State.Transactions[i].Removed).To(BeTrue())
		}
		// Drain ends Paused.
		Expect(comp.State.IsDraining).To(BeFalse())
		Expect(comp.State.IsPaused).To(BeTrue())
	})

	It("does not abort an in-flight Drain when a Pause arrives", func() {
		// Get a read miss in flight and capture its bottom fetch.
		topPort.Deliver(makeRead(0))
		bottomReads := []memprotocol.ReadReq{}
		for i := 0; i < 256 && len(bottomReads) == 0; i++ {
			comp.Tick()
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		Expect(bottomReads).To(HaveLen(1))
		Expect(inflightCount()).To(BeNumerically(">", 0))

		// Begin draining, then a Pause arrives mid-drain. Control commands are
		// serialized, so the Pause stays queued until the drain finishes; it
		// must not freeze the pipeline (which only runs while !IsPaused) and
		// strand the drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		comp.Tick()
		Expect(comp.State.IsDraining).To(BeTrue())

		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)
		comp.Tick()

		// Feed the fill; the in-flight read retires and the drain still acks.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}
		drainAcked, pauseAcked := false, false
		for i := 0; i < 4096 && !drainAcked; i++ {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				r, ok := out.(memcontrolprotocol.Rsp)
				if !ok {
					continue
				}
				switch r.Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		Expect(pauseAcked).To(BeTrue())
		Expect(drainAcked).To(BeTrue())
		Expect(comp.State.IsDraining).To(BeFalse())
		Expect(comp.State.IsPaused).To(BeTrue())
	})

	It("completes a Drain issued while paused with work in flight", func() {
		// Get a read miss in flight, then capture its bottom fetch.
		topPort.Deliver(makeRead(0))
		bottomReads := []memprotocol.ReadReq{}
		for i := 0; i < 256 && len(bottomReads) == 0; i++ {
			comp.Tick()
			bottomReads = append(bottomReads, captureBottomReads()...)
		}
		Expect(bottomReads).To(HaveLen(1))
		Expect(inflightCount()).To(BeNumerically(">", 0))

		// Pause: the in-flight transaction freezes (pipeline stops).
		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)
		pausedAck := false
		for i := 0; i < 64 && !pausedAck; i++ {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok && rsp.RspTo == pause.ID {
					Expect(rsp.Success).To(BeTrue())
					pausedAck = true
				}
			}
		}
		Expect(pausedAck).To(BeTrue())
		Expect(comp.State.IsPaused).To(BeTrue())
		Expect(inflightCount()).To(BeNumerically(">", 0))

		// Make the fill available, then Drain from the paused state. The drain
		// must clear the pause so the pipeline resumes, retire the frozen
		// transaction, and ack. Before the fix the pipeline stayed frozen
		// (IsPaused never cleared) and this Drain hung forever.
		for _, br := range bottomReads {
			bottomPort.Deliver(makeFill(br))
		}
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		var drainRsp memcontrolprotocol.Rsp
		found := false
		for i := 0; i < 4096 && !found; i++ {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
					rsp.Command == memcontrolprotocol.CmdDrain {
					drainRsp = rsp
					found = true
				}
			}
		}

		Expect(found).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		for i := range comp.State.Transactions {
			Expect(comp.State.Transactions[i].Removed).To(BeTrue())
		}
		Expect(comp.State.IsDraining).To(BeFalse())
		Expect(comp.State.IsPaused).To(BeTrue())
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.IsPaused = true
		topPort.Deliver(makeRead(0))

		for range 5 {
			comp.Tick()
		}

		// The request is neither consumed nor turned into work, and nothing
		// is forwarded out the Bottom port, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(inflightCount()).To(Equal(0))
		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(setStart func()) {
			// Get a read miss in flight.
			topPort.Deliver(makeRead(0))
			for i := 0; i < 256 && inflightCount() == 0; i++ {
				comp.Tick()
			}
			Expect(inflightCount()).To(BeNumerically(">", 0))

			setStart()

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp memcontrolprotocol.Rsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, found = out.(memcontrolprotocol.Rsp)
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.Transactions).To(BeEmpty())
			Expect(comp.State.IsPaused).To(BeFalse())
			Expect(comp.State.IsDraining).To(BeFalse())
		},
		Entry("from Running", func() {
			comp.State.IsPaused = false
			comp.State.IsDraining = false
		}),
		Entry("from Paused", func() {
			comp.State.IsPaused = true
		}),
		// The "from Draining" case is covered separately by the dedicated test
		// below, because with strict serialization a Reset queued behind an
		// in-progress Drain is only serviced after the Drain acks.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
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

		var rsps []memcontrolprotocol.Rsp
		for range 16 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(memcontrolprotocol.Rsp); ok {
					rsps = append(rsps, r)
				}
			}
		}

		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Command).To(Equal(memcontrolprotocol.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.IsDraining).To(BeFalse())
		Expect(comp.State.IsPaused).To(BeFalse())
	})

	// driveCtrl delivers a control req and ticks until its ControlRsp comes
	// back (or the budget runs out), returning the matching Rsp and whether
	// it was found.
	driveCtrl := func(req memcontrolprotocol.Req) (memcontrolprotocol.Rsp, bool) {
		ctrlPort.Deliver(req)

		for range 64 {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
					rsp.RspTo == req.ID {
					return rsp, true
				}
			}
		}
		return memcontrolprotocol.Rsp{}, false
	}

	It("invalidates only directory blocks matching the address filter", func() {
		// Two distinct, clean, resident cache lines.
		const addrA = uint64(0)
		const addrB = blockSize
		setA, wayA := seedBlock(addrA, vm.PID(1))
		setB, wayB := seedBlock(addrB, vm.PID(1))

		// They must occupy distinct blocks for the filter to be meaningful.
		Expect([2]int{setA, wayA}).ToNot(Equal([2]int{setB, wayB}))

		// Invalidate is only legal once quiesced.
		comp.State.IsPaused = true

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		inv.Addresses = []uint64{addrA}
		inv.PID = vm.PID(1)

		rsp, found := driveCtrl(inv)

		Expect(found).To(BeTrue())
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Success).To(BeTrue())
		Expect(rsp.Error).To(BeEmpty())

		// Only the filtered block A is dropped; block B survives untouched.
		blockA := comp.State.DirectoryState.Sets[setA].Blocks[wayA]
		blockB := comp.State.DirectoryState.Sets[setB].Blocks[wayB]
		Expect(blockA.IsValid).To(BeFalse())
		Expect(blockB.IsValid).To(BeTrue())
		Expect(blockB.Tag).To(Equal(addrB / blockSize * blockSize))
	})

	It("acks Flush while paused without dropping any blocks", func() {
		const addr = uint64(0)
		setID, wayID := seedBlock(addr, vm.PID(2))

		comp.State.IsPaused = true

		rsp, found := driveCtrl(makeCtrlReq(memcontrolprotocol.CmdFlush))

		Expect(found).To(BeTrue())
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdFlush))
		Expect(rsp.Success).To(BeTrue())
		Expect(rsp.Error).To(BeEmpty())

		// Flush is a no-op for writethrough: the clean block is untouched.
		block := comp.State.DirectoryState.Sets[setID].Blocks[wayID]
		Expect(block.IsValid).To(BeTrue())
		Expect(block.Tag).To(Equal(addr / blockSize * blockSize))
	})

	It("rejects Invalidate issued while Enabled", func() {
		setID, wayID := seedBlock(0, vm.PID(1))

		// Component is freshly built: Enabled (not paused, not draining).
		Expect(comp.State.IsPaused).To(BeFalse())
		Expect(comp.State.IsDraining).To(BeFalse())

		inv := makeCtrlReq(memcontrolprotocol.CmdInvalidate)
		inv.Addresses = []uint64{0}

		rsp, found := driveCtrl(inv)

		Expect(found).To(BeTrue())
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Success).To(BeFalse())
		Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrMustBePausedOrDrained))

		// Rejected Invalidate must leave the directory intact.
		block := comp.State.DirectoryState.Sets[setID].Blocks[wayID]
		Expect(block.IsValid).To(BeTrue())
	})
})
