package cache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

func TestMsgRefRoundTrip(t *testing.T) {
	msg := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:           "msg-1",
			Src:          "src-port",
			Dst:          "dst-port",
			TrafficClass: "data",
			TrafficBytes: 64,
		},
		RspTo: "rsp-1",
	}

	ref := MsgRefFromMsg(msg)
	restored := MsgFromRef(ref)

	if restored.ID != msg.ID {
		t.Errorf("ID: got %q, want %q", restored.ID, msg.ID)
	}

	if restored.Src != msg.Src {
		t.Errorf("Src: got %q, want %q", restored.Src, msg.Src)
	}

	if restored.Dst != msg.Dst {
		t.Errorf("Dst: got %q, want %q", restored.Dst, msg.Dst)
	}

	if restored.RspTo != msg.RspTo {
		t.Errorf("RspTo: got %q, want %q", restored.RspTo, msg.RspTo)
	}

	if restored.TrafficClass != msg.TrafficClass {
		t.Errorf("TrafficClass mismatch")
	}

	if restored.TrafficBytes != msg.TrafficBytes {
		t.Errorf("TrafficBytes mismatch")
	}
}

func TestDirectorySnapshotRestore(t *testing.T) {
	vf := NewLRUVictimFinder()
	dir := NewDirectory(2, 4, 64, vf)

	// Modify some blocks.
	set0 := &dir.Sets[0]
	b := set0.Blocks[1]
	b.PID = vm.PID(42)
	b.Tag = 0x1000
	b.IsValid = true
	b.IsDirty = true
	b.ReadCount = 3
	b.IsLocked = true
	b.DirtyMask = []bool{true, false, true, false}

	// Move block 1 to end of LRU (visit it).
	dir.Visit(b)

	// Take snapshot.
	ds := SnapshotDirectory(dir)

	// Verify ValidateState passes.
	if err := modeling.ValidateState(ds); err != nil {
		t.Fatalf("ValidateState(DirectoryState) failed: %v", err)
	}

	// Build a fresh directory and restore.
	dir2 := NewDirectory(2, 4, 64, vf)
	RestoreDirectory(dir2, ds)

	// Verify the modified block.
	b2 := dir2.Sets[0].Blocks[1]

	if b2.PID != vm.PID(42) {
		t.Errorf("PID: got %d, want 42", b2.PID)
	}

	if b2.Tag != 0x1000 {
		t.Errorf("Tag: got %d, want 0x1000", b2.Tag)
	}

	if !b2.IsValid {
		t.Error("expected IsValid true")
	}

	if !b2.IsDirty {
		t.Error("expected IsDirty true")
	}

	if b2.ReadCount != 3 {
		t.Errorf("ReadCount: got %d, want 3", b2.ReadCount)
	}

	if !b2.IsLocked {
		t.Error("expected IsLocked true")
	}

	if len(b2.DirtyMask) != 4 {
		t.Fatalf("DirtyMask length: got %d, want 4", len(b2.DirtyMask))
	}

	if b2.DirtyMask[0] != true || b2.DirtyMask[1] != false {
		t.Error("DirtyMask values mismatch")
	}

	// Verify LRU order: after visiting block 1, it should be last.
	lruOrder := ds.Sets[0].LRUOrder
	if lruOrder[len(lruOrder)-1] != 1 {
		t.Errorf("LRU last should be wayID 1, got %d",
			lruOrder[len(lruOrder)-1])
	}

	// Verify the LRU queue is restored correctly with pointers.
	lastLRU := dir2.Sets[0].LRUQueue[len(lruOrder)-1]
	if lastLRU != dir2.Sets[0].Blocks[1] {
		t.Error("LRU queue pointer not correctly restored")
	}
}

func TestMSHRSnapshotRestore(t *testing.T) {
	vf := NewLRUVictimFinder()
	dir := NewDirectory(2, 4, 64, vf)
	m := NewMSHR(4)

	// Add an entry.
	entry := m.Add(vm.PID(10), 0x2000)

	// Link to a block.
	entry.Block = dir.Sets[0].Blocks[2]

	// Add some fake transactions.
	trans0 := "transaction-0"
	trans1 := "transaction-1"
	entry.Requests = []interface{}{trans0, trans1}

	// Set ReadReq and DataReady.
	entry.ReadReq = &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  "read-1",
			Src: "cache",
			Dst: "mem",
		},
	}
	entry.DataReady = &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  "data-1",
			Src: "mem",
			Dst: "cache",
		},
	}
	entry.Data = []byte{0xAA, 0xBB, 0xCC}

	// Build transaction lookup.
	transLookup := map[interface{}]int{
		trans0: 0,
		trans1: 1,
	}

	ms := SnapshotMSHR(m, transLookup)

	// Validate state.
	if err := modeling.ValidateState(ms); err != nil {
		t.Fatalf("ValidateState(MSHRState) failed: %v", err)
	}

	// Restore.
	transactions := []interface{}{trans0, trans1}
	m2 := NewMSHR(4)
	RestoreMSHR(m2, ms, transactions, dir)

	entries := m2.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]

	if e.PID != vm.PID(10) {
		t.Errorf("PID: got %d, want 10", e.PID)
	}

	if e.Address != 0x2000 {
		t.Errorf("Address: got %d, want 0x2000", e.Address)
	}

	if len(e.Requests) != 2 {
		t.Fatalf("Requests length: got %d, want 2", len(e.Requests))
	}

	if e.Requests[0] != trans0 || e.Requests[1] != trans1 {
		t.Error("Requests not correctly restored")
	}

	if e.Block == nil {
		t.Fatal("Block should not be nil")
	}

	if e.Block.SetID != 0 || e.Block.WayID != 2 {
		t.Errorf("Block ref: got set=%d way=%d, want set=0 way=2",
			e.Block.SetID, e.Block.WayID)
	}

	if e.ReadReq == nil || e.ReadReq.ID != "read-1" {
		t.Error("ReadReq not correctly restored")
	}

	if e.DataReady == nil || e.DataReady.ID != "data-1" {
		t.Error("DataReady not correctly restored")
	}

	if len(e.Data) != 3 ||
		e.Data[0] != 0xAA ||
		e.Data[1] != 0xBB ||
		e.Data[2] != 0xCC {
		t.Error("Data not correctly restored")
	}
}

func TestMSHRSnapshotEmptyEntries(t *testing.T) {
	m := NewMSHR(4)
	transLookup := map[interface{}]int{}

	ms := SnapshotMSHR(m, transLookup)

	if len(ms.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(ms.Entries))
	}

	if err := modeling.ValidateState(ms); err != nil {
		t.Fatalf("ValidateState failed: %v", err)
	}
}

func TestDirectorySnapshotEmptyBlocks(t *testing.T) {
	vf := NewLRUVictimFinder()
	dir := NewDirectory(1, 2, 64, vf)

	ds := SnapshotDirectory(dir)

	if len(ds.Sets) != 1 {
		t.Fatalf("expected 1 set, got %d", len(ds.Sets))
	}

	if len(ds.Sets[0].Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(ds.Sets[0].Blocks))
	}

	if err := modeling.ValidateState(ds); err != nil {
		t.Fatalf("ValidateState failed: %v", err)
	}

	// Restore to fresh directory.
	dir2 := NewDirectory(1, 2, 64, vf)
	RestoreDirectory(dir2, ds)

	for j, b := range dir2.Sets[0].Blocks {
		if b.IsValid {
			t.Errorf("block %d should not be valid", j)
		}
	}
}
