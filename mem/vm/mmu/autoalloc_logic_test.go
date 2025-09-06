package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// TestAutoPageAllocationLogic tests the core auto page allocation functionality
func TestAutoPageAllocationLogic(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Create MMU with auto page allocation enabled
	mmu := MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true).
		Build("TestMMU")

	middleware := &middleware{Comp: mmu}

	// Test physical page allocation starts at 0
	firstPage := middleware.createDefaultPage(vm.PID(1), 0x1234, 2)
	if firstPage.PAddr != 0 {
		t.Errorf("Expected first physical page to be 0, got 0x%x", firstPage.PAddr)
	}

	// Test second allocation gets next page
	secondPage := middleware.createDefaultPage(vm.PID(1), 0x5678, 2)
	expectedSecondPAddr := uint64(4096) // 1 << 12 (default page size)
	if secondPage.PAddr != expectedSecondPAddr {
		t.Errorf("Expected second physical page to be 0x%x, got 0x%x", expectedSecondPAddr, secondPage.PAddr)
	}

	// Test virtual address alignment
	if firstPage.VAddr != 0x1000 { // Should be aligned to 4KB boundary
		t.Errorf("Expected aligned VAddr 0x1000, got 0x%x", firstPage.VAddr)
	}
	if secondPage.VAddr != 0x5000 { // Should be aligned to 4KB boundary
		t.Errorf("Expected aligned VAddr 0x5000, got 0x%x", secondPage.VAddr)
	}

	// Test page properties
	testPage := firstPage
	if testPage.PID != vm.PID(1) {
		t.Errorf("Expected PID 1, got %v", testPage.PID)
	}
	if testPage.PageSize != 4096 { // Default page size for log2PageSize=12
		t.Errorf("Expected page size 4096, got %v", testPage.PageSize)
	}
	if !testPage.Valid {
		t.Error("Expected page to be valid")
	}
	if testPage.DeviceID != 2 {
		t.Errorf("Expected DeviceID 2, got %v", testPage.DeviceID)
	}
	if !testPage.Unified {
		t.Error("Expected page to be unified")
	}
	if testPage.IsMigrating {
		t.Error("Expected page to not be migrating")
	}
	if testPage.IsPinned {
		t.Error("Expected page to not be pinned")
	}
}

// TestPhysicalPageAllocator tests the physical page allocation algorithm
func TestPhysicalPageAllocator(t *testing.T) {
	engine := sim.NewSerialEngine()

	mmu := MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true).
		WithLog2PageSize(12). // 4KB pages
		Build("TestMMU")

	middleware := &middleware{Comp: mmu}

	// Test multiple allocations to ensure unique physical pages
	allocatedPages := make(map[uint64]bool)
	pageSize := uint64(4096)

	for i := 0; i < 10; i++ {
		page := middleware.createDefaultPage(vm.PID(1), uint64(i*0x1000), 1)
		
		// Check that physical address is unique
		if allocatedPages[page.PAddr] {
			t.Errorf("Physical page 0x%x allocated twice", page.PAddr)
		}
		allocatedPages[page.PAddr] = true

		// Check that physical address is page-aligned
		if page.PAddr%pageSize != 0 {
			t.Errorf("Physical address 0x%x is not page-aligned", page.PAddr)
		}

		// Check that we're getting sequential physical pages
		expectedPAddr := uint64(i) * pageSize
		if page.PAddr != expectedPAddr {
			t.Errorf("Expected physical address 0x%x, got 0x%x", expectedPAddr, page.PAddr)
		}
	}
}

// TestAutoPageAllocationDisabled tests behavior when auto page allocation is disabled
func TestAutoPageAllocationDisabled(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Create MMU with auto page allocation disabled (default)
	mmu := MakeBuilder().
		WithEngine(engine).
		Build("TestMMU")

	if mmu.autoPageAllocation {
		t.Error("Auto page allocation should be disabled by default")
	}
}

// TestAutoPageAllocationEnabled tests that auto page allocation is properly enabled
func TestAutoPageAllocationEnabled(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Create MMU with auto page allocation enabled
	mmu := MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true).
		Build("TestMMU")

	if !mmu.autoPageAllocation {
		t.Error("Auto page allocation should be enabled when set")
	}

	if mmu.nextPhysicalPage != 0 {
		t.Errorf("Next physical page should start at 0, got %d", mmu.nextPhysicalPage)
	}
}