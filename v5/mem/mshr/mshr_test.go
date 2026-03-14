package mshr_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/mshr"
	"github.com/sarchlab/akita/v5/mem/vm"
)

type testEntry struct {
	pid  uint32
	addr uint64
}

func (e testEntry) GetPID() uint32     { return e.pid }
func (e testEntry) GetAddress() uint64 { return e.addr }

func TestFind_ExistingEntry(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
		{pid: 2, addr: 0x200},
	}

	idx, found := mshr.Find(entries, vm.PID(2), 0x200)
	if !found || idx != 1 {
		t.Errorf("expected (1, true), got (%d, %v)", idx, found)
	}
}

func TestFind_NotFound(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
	}

	idx, found := mshr.Find(entries, vm.PID(1), 0x999)
	if found || idx != -1 {
		t.Errorf("expected (-1, false), got (%d, %v)", idx, found)
	}
}

func TestFind_EmptySlice(t *testing.T) {
	var entries []testEntry

	idx, found := mshr.Find(entries, vm.PID(1), 0x100)
	if found || idx != -1 {
		t.Errorf("expected (-1, false), got (%d, %v)", idx, found)
	}
}

func TestIsPresent(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
	}

	if !mshr.IsPresent(entries, vm.PID(1), 0x100) {
		t.Error("expected present")
	}

	if mshr.IsPresent(entries, vm.PID(1), 0x200) {
		t.Error("expected not present")
	}
}

func TestIsFull(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
		{pid: 2, addr: 0x200},
	}

	if !mshr.IsFull(entries, 2) {
		t.Error("expected full")
	}

	if mshr.IsFull(entries, 3) {
		t.Error("expected not full")
	}
}

func TestIsEmpty(t *testing.T) {
	var empty []testEntry
	if !mshr.IsEmpty(empty) {
		t.Error("expected empty")
	}

	notEmpty := []testEntry{{pid: 1, addr: 0x100}}
	if mshr.IsEmpty(notEmpty) {
		t.Error("expected not empty")
	}
}

func TestRemove(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
		{pid: 2, addr: 0x200},
		{pid: 3, addr: 0x300},
	}

	result := mshr.Remove(entries, vm.PID(2), 0x200)

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	if result[0].pid != 1 || result[1].pid != 3 {
		t.Errorf("unexpected entries after remove: %v", result)
	}
}

func TestRemove_Panics(t *testing.T) {
	entries := []testEntry{
		{pid: 1, addr: 0x100},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when removing non-existent entry")
		}
	}()

	mshr.Remove(entries, vm.PID(99), 0x999)
}
