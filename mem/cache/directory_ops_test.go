package cache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
)

func TestDirectoryLookup_Found(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	// Place a valid block at set 1, way 0
	ds.Sets[1].Blocks[0].IsValid = true
	ds.Sets[1].Blocks[0].Tag = 64 // addr=64, blockSize=64, setID = (64/64)%4 = 1
	ds.Sets[1].Blocks[0].PID = uint32(vm.PID(5))

	setID, wayID, found := DirectoryLookup(&ds, 4, 64, vm.PID(5), 64)

	if !found {
		t.Fatal("expected to find block")
	}

	if setID != 1 {
		t.Errorf("setID: got %d, want 1", setID)
	}

	if wayID != 0 {
		t.Errorf("wayID: got %d, want 0", wayID)
	}
}

func TestDirectoryLookup_NotFound(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	setID, wayID, found := DirectoryLookup(&ds, 4, 64, vm.PID(1), 64)

	if found {
		t.Fatal("expected not to find block")
	}

	if setID != 1 {
		t.Errorf("setID: got %d, want 1", setID)
	}

	if wayID != -1 {
		t.Errorf("wayID: got %d, want -1", wayID)
	}
}

func TestDirectoryLookup_WrongPID(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	ds.Sets[1].Blocks[0].IsValid = true
	ds.Sets[1].Blocks[0].Tag = 64
	ds.Sets[1].Blocks[0].PID = uint32(vm.PID(5))

	_, _, found := DirectoryLookup(&ds, 4, 64, vm.PID(99), 64)

	if found {
		t.Fatal("expected not to find block with wrong PID")
	}
}

func TestDirectoryLookup_InvalidBlock(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	// Block matches tag+PID but is invalid
	ds.Sets[1].Blocks[0].IsValid = false
	ds.Sets[1].Blocks[0].Tag = 64
	ds.Sets[1].Blocks[0].PID = uint32(vm.PID(5))

	_, _, found := DirectoryLookup(&ds, 4, 64, vm.PID(5), 64)

	if found {
		t.Fatal("expected not to find invalid block")
	}
}

func TestDirectoryFindVictim(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	// Default LRU order: [0, 1], so victim should be way 0
	setID, wayID := DirectoryFindVictim(&ds, 4, 64, 64)

	if setID != 1 {
		t.Errorf("setID: got %d, want 1", setID)
	}

	if wayID != 0 {
		t.Errorf("wayID: got %d, want 0 (LRU)", wayID)
	}
}

func TestDirectoryFindVictim_AfterVisit(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 4, 2, 64)

	// Visit way 0 so it becomes MRU; way 1 becomes LRU
	DirectoryVisit(&ds, 1, 0)

	_, wayID := DirectoryFindVictim(&ds, 4, 64, 64)

	if wayID != 1 {
		t.Errorf("wayID: got %d, want 1 (LRU after visiting 0)", wayID)
	}
}

func TestDirectoryVisit(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 2, 4, 64)

	// Initial LRU order: [0, 1, 2, 3]
	// Visit way 1 → should become MRU
	DirectoryVisit(&ds, 0, 1)

	lru := ds.Sets[0].LRUOrder
	if len(lru) != 4 {
		t.Fatalf("LRU length: got %d, want 4", len(lru))
	}

	if lru[len(lru)-1] != 1 {
		t.Errorf("MRU: got %d, want 1", lru[len(lru)-1])
	}

	if lru[0] != 0 {
		t.Errorf("LRU: got %d, want 0", lru[0])
	}
}

func TestDirectoryVisit_MultipleTimes(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 1, 3, 64)

	// Initial: [0, 1, 2]
	DirectoryVisit(&ds, 0, 0) // [1, 2, 0]
	DirectoryVisit(&ds, 0, 1) // [2, 0, 1]

	lru := ds.Sets[0].LRUOrder
	expected := []int{2, 0, 1}

	for i, v := range expected {
		if lru[i] != v {
			t.Errorf("LRU[%d]: got %d, want %d", i, lru[i], v)
		}
	}
}

func TestDirectoryReset(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 2, 4, 64)

	if len(ds.Sets) != 2 {
		t.Fatalf("expected 2 sets, got %d", len(ds.Sets))
	}

	for i, set := range ds.Sets {
		if len(set.Blocks) != 4 {
			t.Errorf("set %d: expected 4 blocks, got %d", i, len(set.Blocks))
		}

		if len(set.LRUOrder) != 4 {
			t.Errorf("set %d: expected 4 LRUOrder entries, got %d",
				i, len(set.LRUOrder))
		}

		for j, block := range set.Blocks {
			if block.IsValid {
				t.Errorf("set %d block %d: should not be valid", i, j)
			}

			if block.SetID != i {
				t.Errorf("set %d block %d: SetID got %d, want %d",
					i, j, block.SetID, i)
			}

			if block.WayID != j {
				t.Errorf("set %d block %d: WayID got %d, want %d",
					i, j, block.WayID, j)
			}

			expectedAddr := uint64(i*4+j) * 64
			if block.CacheAddress != expectedAddr {
				t.Errorf("set %d block %d: CacheAddress got %d, want %d",
					i, j, block.CacheAddress, expectedAddr)
			}
		}

		for j, wayID := range set.LRUOrder {
			if wayID != j {
				t.Errorf("set %d LRUOrder[%d]: got %d, want %d",
					i, j, wayID, j)
			}
		}
	}
}

func TestDirectoryReset_Overwrite(t *testing.T) {
	var ds DirectoryState
	DirectoryReset(&ds, 2, 2, 64)

	// Modify some state
	ds.Sets[0].Blocks[0].IsValid = true
	ds.Sets[0].Blocks[0].Tag = 999

	// Reset again
	DirectoryReset(&ds, 2, 2, 64)

	if ds.Sets[0].Blocks[0].IsValid {
		t.Error("block should be invalid after reset")
	}

	if ds.Sets[0].Blocks[0].Tag != 0 {
		t.Error("block tag should be zero after reset")
	}
}
