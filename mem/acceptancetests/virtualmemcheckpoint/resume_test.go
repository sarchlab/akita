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
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

func cleanup(sim *simulation.Simulation) {
	sim.Terminate()
	os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
}

// buildSim assembles the full virtualmem hierarchy each time, identically:
// Driver -> AT -> L1 (write-through) -> L2 (write-back) -> MemCtrl, with the
// AT's translation path going AT -> TLB -> L2TLB -> IoMMU over a shared page
// table. Every component, port, connection, and the page table is registered,
// so all of it is part of the checkpoint inventory.
func buildSim() (*simulation.Simulation, *driver) {
	sim := simulation.MakeBuilder().WithoutMonitoring().Build()

	l1Cache, l2Cache, memCtrl := buildMemoryHierarchy(sim)
	ioMMU, itlb, l2TLB := buildTranslationHierarchy(sim)

	atSpec := addresstranslator.DefaultSpec()
	atSpec.Log2PageSize = 12
	atSpec.NumReqPerCycle = 4
	at := addresstranslator.MakeBuilder().
		WithRegistrar(sim).
		WithSpec(atSpec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: l1Cache.GetPortByName("Top").AsRemote(),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: itlb.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("AT")

	d := buildDriver(sim, at.GetPortByName("Top"))

	setupConnection(sim, d, at, itlb, l2TLB, ioMMU, l1Cache, l2Cache, memCtrl)

	return sim, d
}

func buildMemoryHierarchy(s *simulation.Simulation) (
	*modeling.Component[writethroughcache.Spec, writethroughcache.State, writethroughcache.Resources],
	*modeling.Component[writeback.Spec, writeback.State, writeback.Resources],
	*idealmemcontroller.Comp,
) {
	memCtrlSpec := idealmemcontroller.DefaultSpec()
	memCtrlSpec.Capacity = 4 * mem.GB
	memCtrlSpec.Width = 1
	memCtrlSpec.Latency = 100
	memCtrlSpec.CacheLineSize = 64
	memCtrl := idealmemcontroller.MakeBuilder().
		WithRegistrar(s).
		WithSpec(memCtrlSpec).
		Build("MemCtrl")

	l2Spec := writeback.DefaultSpec()
	l2Spec.WayAssociativity = 4
	l2Spec.NumReqPerCycle = 2
	l2Spec.AddressMapperType = "single"
	l2Cache := writeback.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2Spec).
		WithResources(writeback.Resources{
			RemotePorts: []messaging.RemotePort{
				memCtrl.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L2Cache")

	l1Spec := writethroughcache.DefaultSpec()
	l1Spec.WritePolicyType = "write-through"
	l1Spec.WayAssociativity = 2
	l1Spec.AddressMapperType = "single"
	l1Cache := writethroughcache.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l1Spec).
		WithResources(writethroughcache.Resources{
			RemotePorts: []messaging.RemotePort{
				l2Cache.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L1Cache")

	return l1Cache, l2Cache, memCtrl
}

func buildTranslationHierarchy(s *simulation.Simulation) (*mmu.Comp, *tlb.Comp, *tlb.Comp) {
	pageTable := setupPageTable(s)

	mmuSpec := mmu.DefaultSpec()
	mmuSpec.Log2PageSize = 12
	mmuSpec.MaxRequestsInFlight = 16
	mmuSpec.Latency = 10
	ioMMU := mmu.MakeBuilder().
		WithRegistrar(s).
		WithSpec(mmuSpec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("IoMMU")

	l2TLBSpec := tlb.DefaultSpec()
	l2TLBSpec.NumWays = 64
	l2TLBSpec.NumSets = 64
	l2TLBSpec.Log2PageSize = 12
	l2TLBSpec.NumReqPerCycle = 4
	l2TLB := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(l2TLBSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: ioMMU.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("L2TLB")

	tlbSpec := tlb.DefaultSpec()
	tlbSpec.NumWays = 8
	tlbSpec.NumSets = 8
	tlbSpec.Log2PageSize = 12
	tlbSpec.NumReqPerCycle = 2
	itlb := tlb.MakeBuilder().
		WithRegistrar(s).
		WithSpec(tlbSpec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: l2TLB.GetPortByName("Top").AsRemote(),
			},
		}).
		Build("TLB")

	return ioMMU, itlb, l2TLB
}

func setupPageTable(s *simulation.Simulation) vm.PageTable {
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

func connect(s *simulation.Simulation, name string, p1, p2 messaging.Port) {
	conn := directconnection.MakeBuilder().WithRegistrar(s).Build(name)
	conn.PlugIn(p1)
	conn.PlugIn(p2)
}

func setupConnection(
	s *simulation.Simulation,
	d *driver,
	at, itlb, l2TLB, ioMMU, l1Cache, l2Cache, memCtrl messaging.Component,
) {
	connect(s, "Conn1", d.GetPortByName("Mem"), at.GetPortByName("Top"))
	connect(s, "Conn2", at.GetPortByName("Translation"), itlb.GetPortByName("Top"))
	connect(s, "Conn3", itlb.GetPortByName("Bottom"), l2TLB.GetPortByName("Top"))
	connect(s, "Conn4", l2TLB.GetPortByName("Bottom"), ioMMU.GetPortByName("Top"))
	connect(s, "Conn5", at.GetPortByName("Bottom"), l1Cache.GetPortByName("Top"))
	connect(s, "Conn6", l1Cache.GetPortByName("Bottom"), l2Cache.GetPortByName("Top"))
	connect(s, "Conn7", l2Cache.GetPortByName("Bottom"), memCtrl.GetPortByName("Top"))
}

// TestVirtualMemHierarchyCompletes validates the assembly and the deterministic
// driver end-to-end (no checkpoint): every written value reads back through the
// translation + cache hierarchy.
func TestVirtualMemHierarchyCompletes(t *testing.T) {
	sim, d := buildSim()
	defer cleanup(sim)

	engine := sim.GetEngine().(*timing.SerialEngine)
	d.TickLater()
	if err := engine.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !d.done() {
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

	sim, d := buildSim()
	defer cleanup(sim)

	engine := sim.GetEngine().(*timing.SerialEngine)
	d.TickLater()
	if err := engine.Run(); err != nil {
		t.Fatalf("reference run: %v", err)
	}
	if !d.done() {
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

	sim, d := buildSim()
	defer cleanup(sim)

	engine := sim.GetEngine().(*timing.SerialEngine)
	if err := sim.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if err := engine.Run(); err != nil {
		t.Fatalf("resumed run: %v", err)
	}

	if !d.done() {
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
	sim, d := buildSim()
	engine := sim.GetEngine().(*timing.SerialEngine)
	d.TickLater()

	step := wantTime / 8
	if step == 0 {
		step = 1
	}
	for boundary := step; boundary < wantTime; boundary += step {
		if err := engine.RunUntil(boundary); err != nil {
			t.Fatalf("RunUntil: %v", err)
		}
		if d.inFlight() > 0 {
			break
		}
	}
	if d.inFlight() == 0 || d.done() {
		t.Fatalf("never reached a mid-transaction boundary: %+v", d.State)
	}
	t.Logf("checkpoint at t=%d: %d driver requests in flight, writesAcked=%d",
		engine.CurrentTime(), d.inFlight(), d.State.WritesAcked)

	if err := sim.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	cleanup(sim)

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

			sim, d := buildSim()
			engine := sim.GetEngine().(*timing.SerialEngine)
			d.TickLater()
			if err := engine.RunUntil(boundary); err != nil {
				t.Fatalf("RunUntil(%d): %v", boundary, err)
			}
			if err := sim.SaveCheckpoint(path, buildID); err != nil {
				t.Fatalf("SaveCheckpoint: %v", err)
			}
			cleanup(sim)

			resumeAndVerify(t, path, buildID, wantVerified, wantTime)
		})
	}
}
