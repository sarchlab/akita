package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestAutoPageAllocationLogic tests the core auto page allocation functionality
func TestAutoPageAllocationLogic(t *testing.T) {
	engine := timing.NewSerialEngine()

	// Create MMU with auto page allocation enabled
	spec := DefaultSpec()
	spec.AutoPageAllocation = true
	reg := modeling.NewStandaloneRegistrar(engine)
	mmu := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		Build("TestMMU")
	assignPort(reg, mmu, "Top", 4096)
	assignPort(reg, mmu, "Control", 4)

	mw := mmu.Middlewares()[1].(*translationMW)

	// Test physical page allocation starts at 0
	firstPage := mw.createDefaultPage(vm.PID(1), 0x1234, 2)
	if firstPage.PAddr != 0 {
		t.Errorf("Expected first physical page to be 0, got 0x%x", firstPage.PAddr)
	}

	// Test second allocation gets next page
	secondPage := mw.createDefaultPage(vm.PID(1), 0x5678, 2)
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
	engine := timing.NewSerialEngine()

	spec := DefaultSpec()
	spec.AutoPageAllocation = true
	spec.Log2PageSize = 12 // 4KB pages
	reg := modeling.NewStandaloneRegistrar(engine)
	mmu := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		Build("TestMMU")
	assignPort(reg, mmu, "Top", 4096)
	assignPort(reg, mmu, "Control", 4)

	mw := mmu.Middlewares()[1].(*translationMW)

	// Test multiple allocations to ensure unique physical pages
	allocatedPages := make(map[uint64]bool)
	pageSize := uint64(4096)

	for i := range 10 {
		page := mw.createDefaultPage(vm.PID(1), uint64(i*0x1000), 1)

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
	engine := timing.NewSerialEngine()

	// Create MMU with auto page allocation disabled (default)
	reg := modeling.NewStandaloneRegistrar(engine)
	mmu := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(DefaultSpec()).
		Build("TestMMU")
	assignPort(reg, mmu, "Top", 4096)
	assignPort(reg, mmu, "Control", 4)

	if mmu.Spec().AutoPageAllocation {
		t.Error("Auto page allocation should be disabled by default")
	}
}

// TestAutoPageAllocationEnabled tests that auto page allocation is properly enabled
func TestAutoPageAllocationEnabled(t *testing.T) {
	engine := timing.NewSerialEngine()

	// Create MMU with auto page allocation enabled
	spec := DefaultSpec()
	spec.AutoPageAllocation = true
	reg := modeling.NewStandaloneRegistrar(engine)
	mmu := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		Build("TestMMU")
	assignPort(reg, mmu, "Top", 4096)
	assignPort(reg, mmu, "Control", 4)

	if !mmu.Spec().AutoPageAllocation {
		t.Error("Auto page allocation should be enabled when set")
	}

	state := mmu.State
	if state.NextPhysicalPage != 0 {
		t.Errorf("Next physical page should start at 0, got %d",
			state.NextPhysicalPage)
	}
}
