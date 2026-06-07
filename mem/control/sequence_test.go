package control_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-3 cross-component sequence tests: it drives a real
// memory agent through a full sequence of control verbs (the kind a caller
// composes now that the protocol dropped PauseAfter/InvalidateAfter
// modifiers) and asserts the externally observable behavior is correct.

type ticker interface {
	Tick() bool
}

// driveCtrl delivers a control verb and ticks until its ack returns,
// returning the ack. It fails the test if no ack arrives.
func driveCtrl(
	t *testing.T,
	comp ticker,
	ctrl messaging.Port,
	cmd mem.ControlCommand,
	addrs []uint64,
	pid vm.PID,
) mem.ControlRsp {
	t.Helper()

	req := mem.ControlReq{Command: cmd, Addresses: addrs, PID: pid}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("Cmd")
	req.Dst = ctrl.AsRemote()
	req.TrafficClass = "mem.ControlReq"
	ctrl.Deliver(req)

	for range 256 {
		comp.Tick()
		if out := ctrl.RetrieveOutgoing(); out != nil {
			if rsp, ok := out.(mem.ControlRsp); ok && rsp.Command == cmd {
				return rsp
			}
		}
	}

	t.Fatalf("no ack received for %v", cmd)
	return mem.ControlRsp{}
}

// TestTLBSequence_PauseInvalidateEnable exercises the canonical TLB control
// sequence: cache two translations, then Pause -> Invalidate(filtered by
// address) -> Enable, and confirm only the filtered entry was dropped (it
// now misses to Bottom) while the other still hits.
func TestTLBSequence_PauseInvalidateEnable(t *testing.T) {
	engine := timing.NewSerialEngine()
	remote := messaging.RemotePort("MMU")

	reg := modeling.NewStandaloneRegistrar(engine)
	comp := tlb.MakeBuilder().
		WithRegistrar(reg).
		WithSpec(tlb.DefaultSpec()).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{Port: remote},
		}).
		Build("TLB")

	for _, name := range []string{"Top", "Bottom", "Control"} {
		comp.AssignPort(name, modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(comp).
			WithSpec(modeling.PortSpec{BufSize: 16}).
			Build(name))
	}

	top := comp.GetPortByName("Top")
	bottom := comp.GetPortByName("Bottom")
	ctrl := comp.GetPortByName("Control")
	for _, p := range []messaging.Port{top, bottom, ctrl} {
		(&noopConn{}).PlugIn(p)
	}

	const pid = vm.PID(1)

	// Warm the TLB: resolve two pages so both are cached.
	resolveTranslation(t, comp, top, bottom, remote, 0x1000, pid)
	resolveTranslation(t, comp, top, bottom, remote, 0x2000, pid)
	if missed := lookupMisses(t, comp, top, bottom, 0x1000, pid); missed {
		t.Fatal("0x1000 should hit after warming, but it missed")
	}
	if missed := lookupMisses(t, comp, top, bottom, 0x2000, pid); missed {
		t.Fatal("0x2000 should hit after warming, but it missed")
	}

	// Pause -> Invalidate(0x1000) -> Enable.
	if rsp := driveCtrl(t, comp, ctrl, mem.CmdPause, nil, 0); !rsp.Success {
		t.Fatalf("Pause failed: %q", rsp.Error)
	}
	if rsp := driveCtrl(
		t, comp, ctrl, mem.CmdInvalidate, []uint64{0x1000}, pid,
	); !rsp.Success {
		t.Fatalf("Invalidate failed: %q", rsp.Error)
	}
	if rsp := driveCtrl(t, comp, ctrl, mem.CmdEnable, nil, 0); !rsp.Success {
		t.Fatalf("Enable failed: %q", rsp.Error)
	}

	// The invalidated page now misses; the untouched page still hits.
	if missed := lookupMisses(t, comp, top, bottom, 0x1000, pid); !missed {
		t.Error("0x1000 should miss after Invalidate, but it hit")
	}
	if missed := lookupMisses(t, comp, top, bottom, 0x2000, pid); missed {
		t.Error("0x2000 should still hit after Invalidate(0x1000), but missed")
	}
}

// resolveTranslation drives a miss-then-fill so the page becomes cached.
func resolveTranslation(
	t *testing.T,
	comp ticker,
	top, bottom messaging.Port,
	remote messaging.RemotePort,
	vAddr uint64,
	pid vm.PID,
) {
	t.Helper()

	top.Deliver(makeTransReq(top, vAddr, pid))

	var botReq vm.TranslationReq
	botFound := false
	for i := 0; i < 64 && !botFound; i++ {
		comp.Tick()
		if out := bottom.RetrieveOutgoing(); out != nil {
			if r, ok := out.(vm.TranslationReq); ok {
				botReq = r
				botFound = true
			}
		}
	}
	if !botFound {
		t.Fatalf("TLB did not forward a miss for %#x to Bottom", vAddr)
	}

	rsp := vm.TranslationRsp{Page: vm.Page{
		PID: pid, VAddr: vAddr, PAddr: vAddr + 0x10000, Valid: true,
	}}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = remote
	rsp.Dst = bottom.AsRemote()
	rsp.RspTo = botReq.ID
	rsp.TrafficClass = "vm.TranslationRsp"
	bottom.Deliver(rsp)

	for range 64 {
		comp.Tick()
		if out := top.RetrieveOutgoing(); out != nil {
			if _, ok := out.(vm.TranslationRsp); ok {
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
	comp ticker,
	top, bottom messaging.Port,
	vAddr uint64,
	pid vm.PID,
) bool {
	t.Helper()

	top.Deliver(makeTransReq(top, vAddr, pid))

	for range 64 {
		comp.Tick()
		if out := bottom.RetrieveOutgoing(); out != nil {
			if _, ok := out.(vm.TranslationReq); ok {
				return true
			}
		}
		if out := top.RetrieveOutgoing(); out != nil {
			if _, ok := out.(vm.TranslationRsp); ok {
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

	setA := installDirtyBlock(t, comp, storage, 0x0, 0xAA)
	setB := installDirtyBlock(t, comp, storage, 0x40, 0xBB)

	// 1. Drain: the cache holds no in-flight work, so it quiesces and acks.
	if rsp := driveCtrl(t, comp, ctrl, mem.CmdDrain, nil, 0); !rsp.Success {
		t.Fatalf("Drain failed: %q", rsp.Error)
	}

	// 2. Flush (no filter): both dirty blocks are written back to Bottom.
	writtenBack := driveFlushAll(t, comp, ctrl, bottom)
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
	for bottom.RetrieveOutgoing() != nil {
	}

	// 3. Invalidate (no filter): every block is dropped, with no write-back.
	if rsp := driveCtrl(t, comp, ctrl, mem.CmdInvalidate, nil, 0); !rsp.Success {
		t.Fatalf("Invalidate failed: %q", rsp.Error)
	}
	if comp.State.DirectoryState.Sets[setA].Blocks[0].IsValid ||
		comp.State.DirectoryState.Sets[setB].Blocks[0].IsValid {
		t.Error("blocks should be invalid after Invalidate")
	}
	if out := bottom.RetrieveOutgoing(); out != nil {
		t.Errorf("Invalidate must not write back; got %T on Bottom", out)
	}

	// 4. Reset: back to a freshly-built shape (no in-flight transactions).
	if rsp := driveCtrl(t, comp, ctrl, mem.CmdReset, nil, 0); !rsp.Success {
		t.Fatalf("Reset failed: %q", rsp.Error)
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
) vm.TranslationReq {
	req := vm.TranslationReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("Agent")
	req.Dst = top.AsRemote()
	req.PID = pid
	req.VAddr = vAddr
	req.DeviceID = 1
	req.TrafficClass = "vm.TranslationReq"
	return req
}

// driveFlushAll issues an unfiltered Flush, answers every write-back on
// the bottom port, and returns the set of first-data-bytes written back,
// once the async Flush ack arrives. It fails the test if Flush never acks.
func driveFlushAll(
	t *testing.T,
	comp ticker,
	ctrl, bottom messaging.Port,
) map[byte]bool {
	t.Helper()

	flush := mem.ControlReq{Command: mem.CmdFlush}
	flush.ID = timing.GetIDGenerator().Generate()
	flush.Src = messaging.RemotePort("Cmd")
	flush.Dst = ctrl.AsRemote()
	flush.TrafficClass = "mem.ControlReq"
	ctrl.Deliver(flush)

	writtenBack := map[byte]bool{}
	for range 2048 {
		comp.Tick()
		answerWriteBacks(bottom, writtenBack)
		if out := ctrl.RetrieveOutgoing(); out != nil {
			rsp, ok := out.(mem.ControlRsp)
			if ok && rsp.Command == mem.CmdFlush {
				if !rsp.Success {
					t.Fatalf("Flush failed: %q", rsp.Error)
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
		out := bottom.RetrieveOutgoing()
		if out == nil {
			return
		}
		w, ok := out.(mem.WriteReq)
		if !ok {
			continue
		}
		if len(w.Data) > 0 {
			writtenBack[w.Data[0]] = true
		}
		done := mem.WriteDoneRsp{}
		done.ID = timing.GetIDGenerator().Generate()
		done.Src = messaging.RemotePort("LowerCache")
		done.Dst = bottom.AsRemote()
		done.RspTo = w.ID
		done.TrafficClass = "mem.WriteDoneRsp"
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
	storage := mem.NewStorage(1 * mem.MB)

	spec := writeback.DefaultSpec()
	spec.TotalByteSize = 64 * 1024
	spec.NumBanks = 1
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.WayAssociativity = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1

	reg := modeling.NewStandaloneRegistrar(engine)
	comp := writeback.MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(writeback.Resources{
			Storage: storage,
			AddressToPortMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("LowerCache"),
			},
		}).
		Build("L1Cache")

	for _, name := range []string{"Top", "Bottom", "Control"} {
		comp.AssignPort(name, modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(comp).
			WithSpec(modeling.PortSpec{BufSize: 16}).
			Build(name))
	}

	bottom := comp.GetPortByName("Bottom")
	ctrl := comp.GetPortByName("Control")
	for _, p := range []messaging.Port{
		comp.GetPortByName("Top"), bottom, ctrl,
	} {
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
	setID := int(addr / blockSize % uint64(comp.Spec().NumSets))
	block := &comp.State.DirectoryState.Sets[setID].Blocks[0]
	block.Tag = addr
	block.PID = 0
	block.IsValid = true
	block.IsDirty = true

	data := make([]byte, blockSize)
	for i := range data {
		data[i] = fill
	}
	if err := storage.Write(block.CacheAddress, data); err != nil {
		t.Fatalf("seed storage: %v", err)
	}

	return setID
}
