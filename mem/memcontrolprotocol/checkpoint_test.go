package memcontrolprotocol_test

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// This file is the Layer-3 end-to-end checkpoint-correctness test. It wires
// a real write-back cache on top of a real ideal memory controller (the
// backing store) and drives the canonical quiesce-and-snapshot sequence a
// checkpointer uses:
//
//	run a write workload (cache fills with dirty data)
//	  -> Drain   (quiesce: no in-flight work)
//	  -> Flush   (persist: every dirty block written back to the backing memory)
//	  -> Reset   (clean slate: directory emptied)
//	  -> read every address back through the now-empty cache
//
// It then asserts the two correctness guarantees the protocol exists to
// provide: after Drain+Flush the backing memory is a complete, correct
// snapshot of everything written, and a Reset cache still serves correct
// data (by re-fetching it from that snapshot). The reference model is the
// set of writes the test issued — i.e. the no-control baseline.

const cpBlockSize = 64 // 1 << Log2BlockSize(6)

// cacheOverDRAM is a write-back cache wired to an ideal memory controller
// by a hand-rolled message ferry, so the test has full per-tick control
// (the engine is never run) while the backing store is a real component.
type cacheOverDRAM struct {
	cache       *writeback.Comp
	dram        *idealmemcontroller.Comp
	dramStorage *mem.Storage
	top         messaging.Port
	ctrl        messaging.Port
	bottom      messaging.Port
	dramTop     messaging.Port
	agent       messaging.RemotePort
}

// tick advances both components one step and ferries messages across the
// cache.Bottom <-> dram.Top link in both directions.
func (h *cacheOverDRAM) tick() {
	modelingtest.Tick(h.cache)
	modelingtest.Tick(h.dram)

	for {
		m, ok := h.bottom.RetrieveOutgoing()
		if !ok {
			break
		}
		h.dramTop.Deliver(m)
	}
	for {
		m, ok := h.dramTop.RetrieveOutgoing()
		if !ok {
			break
		}
		h.bottom.Deliver(m)
	}
}

func buildCacheOverDRAM(t *testing.T) *cacheOverDRAM {
	t.Helper()

	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	dramStorage := mem.NewStorage(4 * mem.MB)

	dramSpec := idealmemcontroller.Definition.DefaultSpec
	dramSpec.Latency = 5
	dramSpec.Width = 4
	dram := idealmemcontroller.Definition.Builder().
		WithSimulation(sim).
		WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
		WithSpec(dramSpec).
		Build("DRAM")

	dram.BindPort("Top", twowaybuffered.NewPort(16, 16))
	dram.BindPort("Control", twowaybuffered.NewPort(16, 16))

	dramTop := dram.Ports.Top

	cacheSpec := writeback.Definition.DefaultSpec
	cacheSpec.TotalByteSize = 4 * mem.KB
	cacheSpec.Log2BlockSize = 6
	cacheSpec.WayAssociativity = 4
	cacheSpec.NumMSHREntry = 8
	cacheSpec.NumReqPerCycle = 4
	cache := writeback.Definition.Builder().
		WithSimulation(sim).
		WithSpec(cacheSpec).
		WithResources(writeback.Resources{
			Storage: mem.NewStorage(cacheSpec.TotalByteSize),
			AddressToPortMapper: &mem.SinglePortMapper{
				Port: dramTop.AsRemote(),
			},
		}).
		Build("Cache")

	cache.BindPort("Top", twowaybuffered.NewPort(256, 256))
	cache.BindPort("Bottom", twowaybuffered.NewPort(256, 256))
	cache.BindPort("Control", twowaybuffered.NewPort(16, 16))

	h := &cacheOverDRAM{
		cache:       cache,
		dram:        dram,
		dramStorage: dramStorage,
		top:         cache.Ports.Top,
		ctrl:        cache.Ports.Control,
		bottom:      cache.Ports.Bottom,
		dramTop:     dramTop,
		agent:       messaging.RemotePort("Agent"),
	}
	for _, p := range []messaging.Port{h.top, h.ctrl, h.bottom, dramTop} {
		(&noopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	return h
}

// write issues a 4-byte write and ticks until its WriteDoneRsp returns.
func (h *cacheOverDRAM) write(t *testing.T, addr uint64, data []byte) {
	t.Helper()

	req := messaging.Msg{Payload: memprotocol.WriteReq{Address: addr, Data: data},
		ID:           h.cache.NewID(),
		Src:          h.agent,
		Dst:          h.top.AsRemote(),
		TrafficClass: "memprotocol.WriteReq"}

	h.top.Deliver(req)

	for range 4096 {
		h.tick()
		if out, ok := h.top.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(memprotocol.WriteDoneRsp); ok {
				return
			}
		}
	}
	t.Fatalf("write to %#x never completed", addr)
}

// read issues a len(want)-byte read and ticks until the DataReadyRsp
// returns, then returns the data it carried.
func (h *cacheOverDRAM) read(t *testing.T, addr uint64, size uint64) []byte {
	t.Helper()

	req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: size},
		ID:           h.cache.NewID(),
		Src:          h.agent,
		Dst:          h.top.AsRemote(),
		TrafficClass: "memprotocol.ReadReq"}

	h.top.Deliver(req)

	for range 4096 {
		h.tick()
		if out, ok := h.top.RetrieveOutgoing(); ok {
			if rsp, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
				return rsp.Data
			}
		}
	}
	t.Fatalf("read of %#x never completed", addr)
	return nil
}

// control issues a control verb and ticks until its ack, returning it.
func (h *cacheOverDRAM) control(t *testing.T, cmd memcontrolprotocol.Command) messaging.Msg {
	t.Helper()

	req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
		ID:           h.cache.NewID(),
		Src:          h.agent,
		Dst:          h.ctrl.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	h.ctrl.Deliver(req)

	for range 4096 {
		h.tick()
		if out, ok := h.ctrl.RetrieveOutgoing(); ok {
			if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.Command == cmd {
				return out
			}
		}
	}
	t.Fatalf("control verb %v never acked", cmd)
	return messaging.Msg{Payload: memcontrolprotocol.Rsp{}}
}

func TestCheckpoint_DrainFlushReset_PersistsAndServesCorrectData(t *testing.T) {
	h := buildCacheOverDRAM(t)

	const n = 12
	// Reference model: distinct blocks, each with a recognizable 4-byte
	// payload. This is the no-control baseline of what memory should hold.
	want := map[uint64][]byte{}
	for i := range n {
		addr := uint64(i) * cpBlockSize
		data := []byte{byte(i + 1), 0xAB, 0xCD, byte(0xF0 + i)}
		want[addr] = data
		h.write(t, addr, data)
	}

	// Quiesce, then persist all dirty data to the backing memory.
	if rsp := h.control(t, memcontrolprotocol.CmdDrain); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Drain failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if rsp := h.control(t, memcontrolprotocol.CmdFlush); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Flush failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}

	// Guarantee 1: after Drain+Flush the backing memory is a complete,
	// correct snapshot of everything written.
	for addr, data := range want {
		got := h.dramStorage.Read(addr, uint64(len(data)))
		if !bytes.Equal(got, data) {
			t.Errorf("after Flush, backing memory[%#x] = %v, want %v",
				addr, got, data)
		}
	}

	// Reset to a clean slate and confirm the directory is actually empty.
	if rsp := h.control(t, memcontrolprotocol.CmdReset); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Reset failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	for si := range h.cache.State.DirectoryState.Sets {
		for _, b := range h.cache.State.DirectoryState.Sets[si].Blocks {
			if b.IsValid {
				t.Fatalf("Reset left a valid block in set %d", si)
			}
		}
	}

	// Guarantee 2: the reset (empty) cache still serves correct data by
	// re-fetching it from the backing snapshot.
	for addr, data := range want {
		got := h.read(t, addr, uint64(len(data)))
		if !bytes.Equal(got, data) {
			t.Errorf("after Reset, read[%#x] = %v, want %v", addr, got, data)
		}
	}
}

// TestFlush_DoesNotStrandTransactions_AllowingLaterDrain is a regression
// test. A Flush writes every dirty block back through the write buffer;
// those flush-eviction transactions used to be finalized (removed from the
// in-flight eviction list) but never marked Removed, so they lingered in
// the transaction table forever. The flush itself still acked, but the
// next Drain could never observe quiescence and hung indefinitely. This
// drives Flush and then a subsequent Drain, which must ack.
func TestFlush_DoesNotStrandTransactions_AllowingLaterDrain(t *testing.T) {
	h := buildCacheOverDRAM(t)

	const n = 6
	for i := range n {
		h.write(t, uint64(i)*cpBlockSize, []byte{byte(i), 1, 2, 3})
	}

	// Pause, then Flush every dirty block back to the backing memory.
	if rsp := h.control(t, memcontrolprotocol.CmdPause); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Pause failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	if rsp := h.control(t, memcontrolprotocol.CmdFlush); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Flush failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}

	// Resume and run a fresh workload so the following Drain has real work
	// in addition to whatever the flush left behind.
	if rsp := h.control(t, memcontrolprotocol.CmdEnable); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Enable failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
	for i := range n {
		h.write(t, uint64(i)*cpBlockSize, []byte{byte(i), 4, 5, 6})
	}

	// The regression: with flush transactions stranded in the table the
	// cache could never reach quiescence, so this Drain hung (control()
	// would fail "never acked"). It must ack now.
	if rsp := h.control(t, memcontrolprotocol.CmdDrain); !rsp.Payload.(memcontrolprotocol.Rsp).Success {
		t.Fatalf("Drain after Flush failed: %q", rsp.Payload.(memcontrolprotocol.Rsp).Error)
	}
}

// TestReset_DropsOrphanedBottomResponse is a regression test. A Reset issued
// while a fetch is in flight to the lower memory clears the cache's inflight
// correlation metadata; the lower memory's late response then has nothing to
// match. The cache must drop it rather than panic in
// findInflightFetchIdxByFetchReadReqID.
func TestReset_DropsOrphanedBottomResponse(t *testing.T) {
	h := buildCacheOverDRAM(t)

	// A read miss makes the cache issue a fetch out the Bottom port. Tick only
	// the cache (no ferry) and capture that fetch so it stays "outstanding".
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:           h.cache.NewID(),
		Src:          h.agent,
		Dst:          h.top.AsRemote(),
		TrafficClass: "memprotocol.ReadReq"}

	h.top.Deliver(read)

	var fetch messaging.Msg
	for i := 0; i < 4096 && fetch.ID == 0; i++ {
		modelingtest.Tick(h.cache)
		if out, ok := h.bottom.RetrieveOutgoing(); ok {
			fetch = out
			_ = out.Payload.(memprotocol.ReadReq)
		}
	}
	if fetch.ID == 0 {
		t.Fatal("cache never issued a bottom fetch")
	}

	// Reset while the fetch is outstanding (this clears the inflight indices).
	rst := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:           h.cache.NewID(),
		Src:          h.agent,
		Dst:          h.ctrl.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	h.ctrl.Deliver(rst)
	acked := false
	for i := 0; i < 64 && !acked; i++ {
		modelingtest.Tick(h.cache)
		if out, ok := h.ctrl.RetrieveOutgoing(); ok {
			if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
				Command == memcontrolprotocol.CmdReset {
				acked = true
			}
		}
	}
	if !acked {
		t.Fatal("reset never acked")
	}

	// The lower memory's now-orphaned response arrives after the reset.
	rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: make([]byte, cpBlockSize)},
		ID:           h.cache.NewID(),
		Src:          h.dramTop.AsRemote(),
		Dst:          h.bottom.AsRemote(),
		RspTo:        fetch.ID,
		TrafficClass: "memprotocol.DataReadyRsp"}

	h.bottom.Deliver(rsp)

	// Processing the orphan must not panic; it is simply dropped.
	for range 16 {
		modelingtest.Tick(h.cache)
	}

	// The cache still works: a fresh read re-fetches from the backing store.
	if got := h.read(t, 0, 4); len(got) != 4 {
		t.Fatalf("post-reset read returned %d bytes, want 4", len(got))
	}
}
