package cache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
)

func TestMSHRQuery_Found(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
			{PID: uint32(vm.PID(2)), Address: 0x2000},
		},
	}

	idx, found := MSHRQuery(&ms, vm.PID(2), 0x2000)

	if !found {
		t.Fatal("expected to find entry")
	}

	if idx != 1 {
		t.Errorf("idx: got %d, want 1", idx)
	}
}

func TestMSHRQuery_NotFound(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
		},
	}

	idx, found := MSHRQuery(&ms, vm.PID(1), 0x9999)

	if found {
		t.Fatal("expected not to find entry")
	}

	if idx != -1 {
		t.Errorf("idx: got %d, want -1", idx)
	}
}

func TestMSHRQuery_Empty(t *testing.T) {
	ms := MSHRState{}

	idx, found := MSHRQuery(&ms, vm.PID(1), 0x1000)

	if found {
		t.Fatal("expected not to find entry in empty MSHR")
	}

	if idx != -1 {
		t.Errorf("idx: got %d, want -1", idx)
	}
}

func TestMSHRQuery_WrongPID(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
		},
	}

	_, found := MSHRQuery(&ms, vm.PID(2), 0x1000)

	if found {
		t.Fatal("expected not to find entry with wrong PID")
	}
}

func TestMSHRAdd(t *testing.T) {
	ms := MSHRState{}

	idx := MSHRAdd(&ms, 4, vm.PID(10), 0x3000)

	if idx != 0 {
		t.Errorf("idx: got %d, want 0", idx)
	}

	if len(ms.Entries) != 1 {
		t.Fatalf("entries length: got %d, want 1", len(ms.Entries))
	}

	e := ms.Entries[0]
	if vm.PID(e.PID) != vm.PID(10) {
		t.Errorf("PID: got %d, want 10", e.PID)
	}

	if e.Address != 0x3000 {
		t.Errorf("Address: got %d, want 0x3000", e.Address)
	}
}

func TestMSHRAdd_Multiple(t *testing.T) {
	ms := MSHRState{}

	idx0 := MSHRAdd(&ms, 4, vm.PID(1), 0x1000)
	idx1 := MSHRAdd(&ms, 4, vm.PID(2), 0x2000)

	if idx0 != 0 {
		t.Errorf("first idx: got %d, want 0", idx0)
	}

	if idx1 != 1 {
		t.Errorf("second idx: got %d, want 1", idx1)
	}

	if len(ms.Entries) != 2 {
		t.Fatalf("entries length: got %d, want 2", len(ms.Entries))
	}
}

func TestMSHRAdd_PanicWhenFull(t *testing.T) {
	ms := MSHRState{}

	MSHRAdd(&ms, 1, vm.PID(1), 0x1000)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when adding to full MSHR")
		}
	}()

	MSHRAdd(&ms, 1, vm.PID(2), 0x2000)
}

func TestMSHRRemove(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
			{PID: uint32(vm.PID(2)), Address: 0x2000},
			{PID: uint32(vm.PID(3)), Address: 0x3000},
		},
	}

	MSHRRemove(&ms, vm.PID(2), 0x2000)

	if len(ms.Entries) != 2 {
		t.Fatalf("entries length: got %d, want 2", len(ms.Entries))
	}

	// Verify remaining entries
	if vm.PID(ms.Entries[0].PID) != vm.PID(1) || ms.Entries[0].Address != 0x1000 {
		t.Errorf("entry 0 wrong: PID=%d, Addr=0x%x", ms.Entries[0].PID, ms.Entries[0].Address)
	}

	if vm.PID(ms.Entries[1].PID) != vm.PID(3) || ms.Entries[1].Address != 0x3000 {
		t.Errorf("entry 1 wrong: PID=%d, Addr=0x%x", ms.Entries[1].PID, ms.Entries[1].Address)
	}
}

func TestMSHRRemove_PanicWhenNotFound(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when removing non-existent entry")
		}
	}()

	MSHRRemove(&ms, vm.PID(99), 0x9999)
}

func TestMSHRIsFull_False(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
		},
	}

	if MSHRIsFull(&ms, 4) {
		t.Error("expected MSHR not full")
	}
}

func TestMSHRIsFull_True(t *testing.T) {
	ms := MSHRState{
		Entries: []MSHREntryState{
			{PID: uint32(vm.PID(1)), Address: 0x1000},
			{PID: uint32(vm.PID(2)), Address: 0x2000},
		},
	}

	if !MSHRIsFull(&ms, 2) {
		t.Error("expected MSHR full")
	}
}

func TestMSHRIsFull_Empty(t *testing.T) {
	ms := MSHRState{}

	if MSHRIsFull(&ms, 4) {
		t.Error("expected empty MSHR not full")
	}
}

func TestMSHRIsFull_ZeroCapacity(t *testing.T) {
	ms := MSHRState{}

	if !MSHRIsFull(&ms, 0) {
		t.Error("expected MSHR with 0 capacity to be full")
	}
}
