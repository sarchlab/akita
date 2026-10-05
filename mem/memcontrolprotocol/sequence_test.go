package memcontrolprotocol_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-3 cross-component sequence tests: it drives a real
// memory agent through a full sequence of control verbs (the kind a caller
// composes now that the protocol dropped PauseAfter/InvalidateAfter
// modifiers) and asserts the externally observable behavior is correct.

// stepper advances a component by one cycle and reports whether it made
// progress.
type stepper func() bool

// stepperOf returns a stepper that ticks c with modelingtest.Tick.
func stepperOf[S, T, R, P, M any](c *ticking.Component[S, T, R, P, M]) stepper {
	return func() bool { return modelingtest.Tick(c) }
}

// driveCtrl delivers a control verb and ticks until its ack returns,
// returning the ack. It fails the test if no ack arrives.
func driveCtrl(
	t *testing.T,
	tick stepper,
	ctrl messaging.Port,
	cmd memcontrolprotocol.Command,
	addrs []uint64,
	pid vm.PID,
) messaging.Msg {
	t.Helper()

	req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd, Addresses: addrs, PID: pid},
		ID:           newIDFor(ctrl),
		Src:          messaging.RemotePort("Cmd"),
		Dst:          ctrl.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	ctrl.Deliver(req)

	for range 256 {
		tick()
		if out, ok := ctrl.RetrieveOutgoing(); ok {
			if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.Command == cmd {
				return out
			}
		}
	}

	t.Fatalf("no ack received for %v", cmd)
	return messaging.Msg{Payload: memcontrolprotocol.Rsp{}}
}

// TestTLBSequence_PauseInvalidateEnable exercises the canonical TLB control
// sequence: cache two translations, then Pause -> Invalidate(filtered by
// address) -> Enable, and confirm only the filtered entry was dropped (it
// now misses to Bottom) while the other still hits.
func TestTLBSequence_PauseInvalidateEnable(t *testing.T) {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	remote := messaging.RemotePort("MMU")

	comp := tlb.Definition.Builder().
		WithSimulation(sim).
		WithSpec(tlb.Definition.DefaultSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{Port: remote},
		}).
		WithPorts(tlb.Ports{
			Top:     messaging.NewPort("TLB.Top", 16, 16),
			Bottom:  messaging.NewPort("TLB.Bottom", 16, 16),
			Control: messaging.NewPort("TLB.Control", 16, 16),
		}).
		Build("TLB")
	tick := stepperOf(comp)

	top := comp.Ports.Top
	bottom := comp.Ports.Bottom
	ctrl := comp.Ports.Control
	for _, p := range []messaging.Port{top, bottom, ctrl} {
		(&noopConn{}).PlugIn(p)
	}

	const pid = vm.PID(1)

	// Warm the TLB: resolve two pages so both are cached.
	resolveTranslation(t, tick, top, bottom, remote, 0x1000, pid)
	resolveTranslation(t, tick, top, bottom, remote, 0x2000, pid)
	if missed := lookupMisses(t, tick, top, bottom, 0x1000, pid); missed {
		t.Fatal("0x1000 should hit after warming, but it missed")
	}
	if missed := lookupMisses(t, tick, top, bottom, 0x2000, pid); missed {
		t.Fatal("0x2000 should hit after warming, but it missed")
	}

	// Pause -> Invalidate(0x1000) -> Enable.
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdPause, nil, 0,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Pause failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdInvalidate, []uint64{0x1000}, pid,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Invalidate failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdEnable, nil, 0,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Enable failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}

	// The invalidated page now misses; the untouched page still hits.
	if missed := lookupMisses(t, tick, top, bottom, 0x1000, pid); !missed {
		t.Error("0x1000 should miss after Invalidate, but it hit")
	}
	if missed := lookupMisses(t, tick, top, bottom, 0x2000, pid); missed {
		t.Error("0x2000 should still hit after Invalidate(0x1000), but missed")
	}
}

// resolveTranslation drives a miss-then-fill so the page becomes cached.
func resolveTranslation(
	t *testing.T,
	tick stepper,
	top, bottom messaging.Port,
	remote messaging.RemotePort,
	vAddr uint64,
	pid vm.PID,
) {
	t.Helper()

	top.Deliver(makeTransReq(top, vAddr, pid))

	var botReq messaging.Msg
	botFound := false
	for i := 0; i < 64 && !botFound; i++ {
		tick()
		if out, ok := bottom.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(vmprotocol.TranslationReq); ok {
				botReq = out

				botFound = true
			}
		}
	}
	if !botFound {
		t.Fatalf("TLB did not forward a miss for %#x to Bottom", vAddr)
	}

	rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{Page: vm.Page{
		PID: pid, VAddr: vAddr, PAddr: vAddr + 0x10000, Valid: true,
	}},
		ID:           newIDFor(bottom),
		Src:          remote,
		Dst:          bottom.AsRemote(),
		RspTo:        botReq.ID,
		TrafficClass: "vmprotocol.TranslationRsp"}

	bottom.Deliver(rsp)

	for range 64 {
		tick()
		if out, ok := top.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
				return
			}
		}
	}
	t.Fatalf("TLB did not answer the fill for %#x on Top", vAddr)
}

// lookupMisses issues a lookup and reports whether the TLB treated it as a
// miss (forwarded a request out Bottom) versus a hit (answered on Top).
func lookupMisses(
	t *testing.T,
	tick stepper,
	top, bottom messaging.Port,
	vAddr uint64,
	pid vm.PID,
) bool {
	t.Helper()

	top.Deliver(makeTransReq(top, vAddr, pid))

	for range 64 {
		tick()
		if out, ok := bottom.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(vmprotocol.TranslationReq); ok {
				return true
			}
		}
		if out, ok := top.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
				return false
			}
		}
	}

	t.Fatalf("lookup of %#x neither hit nor missed within budget", vAddr)
	return false
}

// TestCacheSequence_DrainFlushInvalidateReset exercises the canonical
// write-back-cache control sequence a checkpointer composes from
// primitives: Drain (quiesce) -> Flush (persist dirty data) ->
// Invalidate (drop clean lines) -> Reset (return to freshly-built). It
// installs two dirty blocks and confirms each verb's externally visible
// effect.
func TestCacheSequence_DrainFlushInvalidateReset(t *testing.T) {
	comp, storage, ctrl, bottom := buildWritebackForSequence(t)
	tick := stepperOf(comp)

	setA := installDirtyBlock(t, comp, storage, 0x0, 0xAA)
	setB := installDirtyBlock(t, comp, storage, 0x40, 0xBB)

	// 1. Drain: the cache holds no in-flight work, so it quiesces and acks.
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdDrain, nil, 0,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Drain failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}

	// 2. Flush (no filter): both dirty blocks are written back to Bottom.
	writtenBack := driveFlushAll(t, tick, ctrl, bottom)
	if !writtenBack[0xAA] || !writtenBack[0xBB] {
		t.Errorf("Flush did not write back both dirty blocks: %v", writtenBack)
	}
	// Flushed blocks stay valid but are now clean.
	if comp.State.DirectoryState.Sets[setA].Blocks[0].IsDirty ||
		comp.State.DirectoryState.Sets[setB].Blocks[0].IsDirty {
		t.Error("blocks should be clean after Flush")
	}
	if !comp.State.DirectoryState.Sets[setA].Blocks[0].IsValid {
		t.Error("flushed block should remain valid")
	}

	// Drain any straggler Bottom traffic before checking Invalidate.
	for {
		if _, ok := bottom.RetrieveOutgoing(); !ok {
			break
		}
	}

	// 3. Invalidate (no filter): every block is dropped, with no write-back.
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdInvalidate, nil, 0,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Invalidate failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if comp.State.DirectoryState.Sets[setA].Blocks[0].IsValid ||
		comp.State.DirectoryState.Sets[setB].Blocks[0].IsValid {
		t.Error("blocks should be invalid after Invalidate")
	}
	if out, ok := bottom.RetrieveOutgoing(); ok {
		t.Errorf("Invalidate must not write back; got %T on Bottom", out)
	}

	// 4. Reset: back to a freshly-built shape (no in-flight transactions).
	if rsp := driveCtrl(
		t, tick, ctrl, memcontrolprotocol.CmdReset, nil, 0,
	); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Reset failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if len(comp.State.Transactions) != 0 {
		t.Errorf("Transactions should be empty after Reset, got %d",
			len(comp.State.Transactions))
	}
}

func makeTransReq(
	top messaging.Port,
	vAddr uint64,
	pid vm.PID,
) messaging.Msg {
	req := messaging.Msg{Payload: vmprotocol.TranslationReq{
		PID:      pid,
		VAddr:    vAddr,
		DeviceID: 1},
		ID:  newIDFor(top),
		Src: messaging.RemotePort("Agent"),
		Dst: top.AsRemote(),

		TrafficClass: "vmprotocol.TranslationReq"}

	return req
}

// driveFlushAll issues an unfiltered Flush, answers every write-back on
// the bottom port, and returns the set of first-data-bytes written back,
// once the async Flush ack arrives. It fails the test if Flush never acks.
func driveFlushAll(
	t *testing.T,
	tick stepper,
	ctrl, bottom messaging.Port,
) map[byte]bool {
	t.Helper()

	flush := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
		ID:           newIDFor(ctrl),
		Src:          messaging.RemotePort("Cmd"),
		Dst:          ctrl.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	ctrl.Deliver(flush)

	writtenBack := map[byte]bool{}
	for range 2048 {
		tick()
		answerWriteBacks(bottom, writtenBack)
		if out, ok := ctrl.RetrieveOutgoing(); ok {
			_, ok := out.Payload.(memcontrolprotocol.Rsp)
			rsp := out
			if ok && rsp.Payload.(memcontrolprotocol.Rsp).Command == memcontrolprotocol.CmdFlush {
				if !rsp.Payload.(memcontrolprotocol.Rsp).Success {
					t.Fatalf("Flush failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
				}
				return writtenBack
			}
		}
	}

	t.Fatal("Flush did not complete within budget")
	return writtenBack
}

// answerWriteBacks drains the bottom port's outgoing write-backs,
// recording each block's first data byte and acking it with a
// WriteDoneRsp so the flush can make progress.
func answerWriteBacks(bottom messaging.Port, writtenBack map[byte]bool) {
	for {
		out, ok := bottom.RetrieveOutgoing()
		if !ok {
			return
		}
		_, ok = out.Payload.(memprotocol.WriteReq)
		w := out
		if !ok {
			continue
		}
		if len(w.Payload.(memprotocol.WriteReq).Data) > 0 {
			writtenBack[w.Payload.(memprotocol.WriteReq).Data[0]] = true
		}
		done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:           newIDFor(bottom),
			Src:          messaging.RemotePort("LowerCache"),
			Dst:          bottom.AsRemote(),
			RspTo:        w.ID,
			TrafficClass: "memprotocol.WriteDoneRsp"}

		bottom.Deliver(done)
	}
}

// buildWritebackForSequence builds a small write-back cache wired with
// noop connections, returning the component, its backing storage, and the
// Control and Bottom ports the sequence test drives.
func buildWritebackForSequence(
	t *testing.T,
) (*writeback.Comp, *mem.Storage, messaging.Port, messaging.Port) {
	t.Helper()

	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	storage := mem.NewStorage(1 * mem.MB)

	spec := writeback.Definition.DefaultSpec
	spec.TotalByteSize = 64 * 1024
	spec.NumBanks = 1
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.WayAssociativity = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1

	comp := writeback.Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(writeback.Resources{
			Storage: storage,
			AddressToPortMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("LowerCache"),
			},
		}).
		WithPorts(writeback.Ports{
			Top:     messaging.NewPort("L1Cache.Top", 16, 16),
			Bottom:  messaging.NewPort("L1Cache.Bottom", 16, 16),
			Control: messaging.NewPort("L1Cache.Control", 16, 16),
		}).
		Build("L1Cache")

	bottom := comp.Ports.Bottom
	ctrl := comp.Ports.Control
	for _, p := range []messaging.Port{comp.Ports.Top, bottom, ctrl} {
		(&noopConn{}).PlugIn(p)
	}

	return comp, storage, ctrl, bottom
}

// installDirtyBlock seats a valid+dirty block holding addr at way 0 of its
// set, writing a recognizable payload (fill bytes) into backing storage so
// a later flush write-back can be identified by its data. Returns the set.
func installDirtyBlock(
	t *testing.T,
	comp *writeback.Comp,
	storage *mem.Storage,
	addr uint64,
	fill byte,
) int {
	t.Helper()

	const blockSize = 64
	numSets := uint64(len(comp.State.DirectoryState.Sets))
	setID := int(addr / blockSize % numSets)
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

// newIDFor allocates an ID from the simulation of the component that owns p,
// for a message the test sends through p.
func newIDFor(p messaging.Port) uint64 {
	return p.Owner().(interface{ NewID() uint64 }).NewID()
}
