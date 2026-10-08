package virtualmemcheckpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func cleanup(s *sim.Simulation) {
	s.Terminate()
	os.Remove("akita_sim_" + s.ID() + ".sqlite3")
}

// buildSim assembles the full virtualmem hierarchy each time, identically:
// Driver -> AT -> L1 (write-through) -> L2 (write-back) -> MemCtrl, with the
// AT's translation path going AT -> TLB -> L2TLB -> IoMMU over a shared page
// table. Every component, port, connection, and the page table is registered,
// so all of it is part of the checkpoint inventory.
func buildSim() (*sim.Simulation, *driver) {
	s := sim.MakeBuilder().Build()

	l1Cache, l2Cache, memCtrl := buildMemoryHierarchy(s)
	ioMMU, itlb, l2TLB := buildTranslationHierarchy(s)

	atSpec := addresstranslator.Definition.DefaultSpec
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4
	at := addresstranslator.Definition.Builder().
		WithSimulation(s).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: l1Cache.Ports.Top.AsRemote(),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: itlb.Ports.Top.AsRemote(),
			},
		}).
		Build("AT")

	at.BindPort("Top", newPort("AT.Top"))
	at.BindPort("Bottom", newPort("AT.Bottom"))
	at.BindPort("Translation", newPort("AT.Translation"))
	at.BindPort("Control", newPort("AT.Control"))

	d := buildDriver(s, at.Ports.Top)

	setupConnection(s, d, at, itlb, l2TLB, ioMMU, l1Cache, l2Cache, memCtrl)

	if err := s.Initialize(); err != nil {
		panic(err)
	}

	return s, d
}

//nolint:funlen // wires the whole hierarchy in one place
func buildMemoryHierarchy(s *sim.Simulation) (
	*writethroughcache.Comp,
	*writeback.Comp,
	*idealmemcontroller.Comp,
) {
	memCtrlSpec := idealmemcontroller.Definition.DefaultSpec
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.Definition.Builder().
		WithSimulation(s).
		WithSpec(memCtrlSpec).
		WithResources(idealmemcontroller.Resources{
			Storage: newStorage(s, 4*mem.GB, "MemCtrl.Storage"),
		}).
		Build("MemCtrl")

	memCtrl.BindPort("Top", newPort("MemCtrl.Top"))
	memCtrl.BindPort("Control", newPort("MemCtrl.Control"))

	l2Spec := writeback.Definition.DefaultSpec
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Spec.AddressMapperType = "single"
	l2Cache := writeback.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			Storage: newStorage(s, l2Spec.TotalByteSize, "L2Cache.Storage"),
			RemotePorts: []messaging.RemotePort{
				memCtrl.Ports.Top.AsRemote(),
			},
		}).
		Build("L2Cache")

	l2Cache.BindPort("Top", newPort("L2Cache.Top"))
	l2Cache.BindPort("Bottom", newPort("L2Cache.Bottom"))
	l2Cache.BindPort("Control", newPort("L2Cache.Control"))

	l1Spec := writethroughcache.Definition.DefaultSpec
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"
	l1Cache := writethroughcache.Definition.Builder().
		WithSimulation(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			Storage: newStorage(s, l1Spec.TotalByteSize, "L1Cache.Storage"),
			RemotePorts: []messaging.RemotePort{
				l2Cache.Ports.Top.AsRemote(),
			},
		}).
		Build("L1Cache")

	l1Cache.BindPort("Top", newPort("L1Cache.Top"))
	l1Cache.BindPort("Bottom", newPort("L1Cache.Bottom"))
	l1Cache.BindPort("Control", newPort("L1Cache.Control"))

	return l1Cache, l2Cache, memCtrl
}

func buildTranslationHierarchy(s *sim.Simulation) (*mmu.Comp, *tlb.Comp, *tlb.Comp) {
	pageTable := setupPageTable(s)

	mmuSpec := mmu.Definition.DefaultSpec
	mmuSpec.Log2PageSize = 12
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	ioMMU := mmu.Definition.Builder().
		WithSimulation(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("IoMMU")

	ioMMU.BindPort("Top", newPort("IoMMU.Top"))
	ioMMU.BindPort("Control", newPort("IoMMU.Control"))

	l2TLBSpec := tlb.Definition.DefaultSpec
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = 12
	l2TLBSpec.NumReqPerCycle = 4
	l2TLB := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: ioMMU.Ports.Top.AsRemote(),
			},
		}).
		Build("L2TLB")

	l2TLB.BindPort("Top", newPort("L2TLB.Top"))
	l2TLB.BindPort("Bottom", newPort("L2TLB.Bottom"))
	l2TLB.BindPort("Control", newPort("L2TLB.Control"))

	tlbSpec := tlb.Definition.DefaultSpec
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2
	itlb := tlb.Definition.Builder().
		WithSimulation(s).
		WithSpec(tlbSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l2TLB.Ports.Top.AsRemote(),
			},
		}).
		Build("TLB")

	itlb.BindPort("Top", newPort("TLB.Top"))
	itlb.BindPort("Bottom", newPort("TLB.Bottom"))
	itlb.BindPort("Control", newPort("TLB.Control"))

	return ioMMU, itlb, l2TLB
}

func setupPageTable(s *sim.Simulation) vm.PageTable {
	pageTable := vm.MakePageTableBuilder().
		WithSimulation(s).
		WithLog2PageSize(12).
		Build("PageTable")

	const ptBase = uint64(0x100000)
	const pageSize = uint64(4096)

	// Map every page the driver can touch (addresses up to numOps*512), plus a
	// margin. With a larger numOps this exceeds the TLB capacity, so the LRU
	// evicts — exercising the lruset state that must round-trip.
	span := uint64(numOps)*512 + pageSize
	numEntries := span/pageSize + 1

	for i := uint64(0); i < numEntries; i++ {
		pageTable.Insert(vm.Page{
			PID:      pid,
			VAddr:    i * pageSize,
			PAddr:    ptBase + i*pageSize,
			PageSize: pageSize,
			Valid:    true,
		})
	}

	return pageTable
}

// newPort creates an unowned port named fullName, for a component that takes
// its ports during wiring. BindPort names, binds, and registers it.
func newPort(fullName string) messaging.Port {
	return twowaybuffered.NewPort(16, 16)
}

// newStorage builds a storage of the given capacity that registers with the
// simulation, so its contents are part of the checkpoint.
func newStorage(
	s *sim.Simulation,
	capacity uint64,
	name string,
) *mem.Storage {
	return mem.MakeStorageBuilder().
		WithCapacity(capacity).
		WithSimulation(s).
		Build(name)
}

func connect(s *sim.Simulation, name string, p1, p2 messaging.Port) {
	conn := direct.NewConnection(name, s, timing.GHz)
	conn.BindPort(p1)
	conn.BindPort(p2)
}

func setupConnection(
	s *sim.Simulation,
	d *driver,
	at *addresstranslator.Comp,
	itlb, l2TLB *tlb.Comp,
	ioMMU *mmu.Comp,
	l1Cache *writethroughcache.Comp,
	l2Cache *writeback.Comp,
	memCtrl *idealmemcontroller.Comp,
) {
	connect(s, "Conn1", d.Ports.Mem, at.Ports.Top)
	connect(s, "Conn2", at.Ports.Translation, itlb.Ports.Top)
	connect(s, "Conn3", itlb.Ports.Bottom, l2TLB.Ports.Top)
	connect(s, "Conn4", l2TLB.Ports.Bottom, ioMMU.Ports.Top)
	connect(s, "Conn5", at.Ports.Bottom, l1Cache.Ports.Top)
	connect(s, "Conn6", l1Cache.Ports.Bottom, l2Cache.Ports.Top)
	connect(s, "Conn7", l2Cache.Ports.Bottom, memCtrl.Ports.Top)
}

// TestVirtualMemHierarchyCompletes validates the assembly and the deterministic
// driver end-to-end (no checkpoint): every written value reads back through the
// translation + cache hierarchy.
func TestVirtualMemHierarchyCompletes(t *testing.T) {
	s, d := buildSim()
	defer cleanup(s)

	engine := s.Engine().(*timing.SerialEngine)
	d.TickLater()
	if err := engine.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !done(d) {
		t.Fatalf("did not finish: %+v", d.State)
	}
	if d.State.ReadsVerified != numOps {
		t.Fatalf("verified %d, want %d", d.State.ReadsVerified, numOps)
	}
}

// runReference runs the full hierarchy uninterrupted and returns the oracle: the
// reads-verified count and end time every resumed run must match.
func runReference(t *testing.T) (wantVerified int, wantTime timing.VTimeInPicoSec) {
	t.Helper()

	s, d := buildSim()
	defer cleanup(s)

	engine := s.Engine().(*timing.SerialEngine)
	d.TickLater()
	if err := engine.Run(); err != nil {
		t.Fatalf("reference run: %v", err)
	}
	if !done(d) {
		t.Fatalf("reference run did not finish: %+v", d.State)
	}

	return d.State.ReadsVerified, engine.CurrentTime()
}

// resumeAndVerify rebuilds the identical hierarchy, loads the checkpoint, runs to
// completion, and asserts it matches the uninterrupted reference exactly.
func resumeAndVerify(
	t *testing.T,
	path, buildID string,
	wantVerified int,
	wantTime timing.VTimeInPicoSec,
) {
	t.Helper()

	s, d := buildSim()
	defer cleanup(s)

	engine := s.Engine().(*timing.SerialEngine)
	if err := s.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if err := engine.Run(); err != nil {
		t.Fatalf("resumed run: %v", err)
	}

	if !done(d) {
		t.Fatalf("resumed run did not finish: %+v", d.State)
	}
	if d.State.Mismatch {
		t.Fatalf("resumed run read stale/incorrect data")
	}
	if d.State.ReadsVerified != wantVerified {
		t.Fatalf("resumed verified %d, want %d", d.State.ReadsVerified, wantVerified)
	}
	if engine.CurrentTime() != wantTime {
		t.Fatalf("resumed end time %d, want %d", engine.CurrentTime(), wantTime)
	}
}

func TestVirtualMemMidTransactionResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "virtualmem-resume"

	wantVerified, wantTime := runReference(t)

	// Advance a fresh sim to a genuinely mid-transaction boundary (requests in
	// flight somewhere in the translation/cache hierarchy), then checkpoint.
	s, d := buildSim()
	engine := s.Engine().(*timing.SerialEngine)
	d.TickLater()

	step := wantTime / 8
	if step == 0 {
		step = 1
	}
	for boundary := step; boundary < wantTime; boundary += step {
		if err := engine.RunUntil(boundary); err != nil {
			t.Fatalf("RunUntil: %v", err)
		}
		if inFlight(d) > 0 {
			break
		}
	}
	if inFlight(d) == 0 || done(d) {
		t.Fatalf("never reached a mid-transaction boundary: %+v", d.State)
	}
	t.Logf("checkpoint at t=%d: %d driver requests in flight, writesAcked=%d",
		engine.CurrentTime(), inFlight(d), d.State.WritesAcked)

	if err := s.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	cleanup(s)

	resumeAndVerify(t, path, buildID, wantVerified, wantTime)
}

func TestVirtualMemResumeAcrossBoundaries(t *testing.T) {
	wantVerified, wantTime := runReference(t)

	const slices = 8
	for i := 1; i < slices; i++ {
		boundary := wantTime * timing.VTimeInPicoSec(i) / slices
		t.Run(fmt.Sprintf("boundary_%d_of_%d", i, slices), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ck.tar.gz")
			const buildID = "virtualmem-multi"

			s, d := buildSim()
			engine := s.Engine().(*timing.SerialEngine)
			d.TickLater()
			if err := engine.RunUntil(boundary); err != nil {
				t.Fatalf("RunUntil(%d): %v", boundary, err)
			}
			if err := s.SaveCheckpoint(path, buildID); err != nil {
				t.Fatalf("SaveCheckpoint: %v", err)
			}
			cleanup(s)

			resumeAndVerify(t, path, buildID, wantVerified, wantTime)
		})
	}
}
