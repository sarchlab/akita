package cache

import "github.com/sarchlab/akita/v5/mem/vm"

// MSHRQuery finds the entry matching pid+addr in MSHRState.
// Returns (entryIdx, found).
func MSHRQuery(ms *MSHRState, pid vm.PID, addr uint64) (int, bool) {
	for i, e := range ms.Entries {
		if vm.PID(e.PID) == pid && e.Address == addr {
			return i, true
		}
	}

	return -1, false
}

// MSHRAdd creates a new entry in MSHRState for pid+addr.
// Panics if the MSHR is already at capacity.
func MSHRAdd(ms *MSHRState, capacity int, pid vm.PID, addr uint64) int {
	if len(ms.Entries) >= capacity {
		panic("MSHR is full")
	}

	entry := MSHREntryState{
		PID:     uint32(pid),
		Address: addr,
	}

	ms.Entries = append(ms.Entries, entry)

	return len(ms.Entries) - 1
}

// MSHRRemove removes the entry matching pid+addr from MSHRState.
// Panics if no such entry exists.
func MSHRRemove(ms *MSHRState, pid vm.PID, addr uint64) {
	for i, e := range ms.Entries {
		if vm.PID(e.PID) == pid && e.Address == addr {
			ms.Entries = append(ms.Entries[:i], ms.Entries[i+1:]...)
			return
		}
	}

	panic("trying to remove non-exist MSHR entry")
}

// MSHRIsFull returns true if the MSHRState has reached its capacity.
func MSHRIsFull(ms *MSHRState, capacity int) bool {
	return len(ms.Entries) >= capacity
}
