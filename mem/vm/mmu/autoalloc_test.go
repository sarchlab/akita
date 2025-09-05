package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// TestAutoPageAllocation tests the auto page allocation feature without mocks
func TestAutoPageAllocation(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Test with auto page allocation disabled (default)
	builder := MakeBuilder().WithEngine(engine)
	mmu := builder.Build("MMU")

	if mmu.autoPageAllocation {
		t.Error("Auto page allocation should be disabled by default")
	}

	// Test with auto page allocation enabled
	builder = MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true)
	mmu = builder.Build("MMUAuto")

	if !mmu.autoPageAllocation {
		t.Error("Auto page allocation should be enabled when set")
	}

	// Test page creation logic
	middleware := &middleware{Comp: mmu}
	page := middleware.createDefaultPage(vm.PID(1), 0x1234, 2)

	if page.PID != vm.PID(1) {
		t.Errorf("Expected PID 1, got %v", page.PID)
	}
	if page.VAddr != 0x1000 { // Should be aligned to 4KB boundary
		t.Errorf("Expected aligned VAddr 0x1000, got 0x%x", page.VAddr)
	}
	if page.PageSize != 4096 { // Default page size for log2PageSize=12
		t.Errorf("Expected page size 4096, got %v", page.PageSize)
	}
	if !page.Valid {
		t.Error("Expected page to be valid")
	}
	if page.DeviceID != 2 {
		t.Errorf("Expected DeviceID 2, got %v", page.DeviceID)
	}
	if !page.Unified {
		t.Error("Expected page to be unified")
	}
	if page.IsMigrating {
		t.Error("Expected page to not be migrating")
	}
	if page.IsPinned {
		t.Error("Expected page to not be pinned")
	}
}

// TestBuilderAutoPageAllocationOption tests the builder option
func TestBuilderAutoPageAllocationOption(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Test that the builder properly sets the option
	builder := MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true)

	if !builder.autoPageAllocation {
		t.Error("Builder should have auto page allocation enabled")
	}

	// Test fluent interface
	builder2 := MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true).
		WithAutoPageAllocation(false)

	if builder2.autoPageAllocation {
		t.Error("Builder should have auto page allocation disabled after setting false")
	}
}

// TestPageAddressAlignment tests address alignment
func TestPageAddressAlignment(t *testing.T) {
	engine := sim.NewSerialEngine()
	builder := MakeBuilder().WithEngine(engine)
	mmu := builder.Build("MMU")
	middleware := &middleware{Comp: mmu}

	testCases := []struct {
		input    uint64
		expected uint64
	}{
		{0x0000, 0x0000},
		{0x1000, 0x1000},
		{0x1001, 0x1000},
		{0x1234, 0x1000},
		{0x1FFF, 0x1000},
		{0x2000, 0x2000},
		{0x2001, 0x2000},
	}

	for _, tc := range testCases {
		page := middleware.createDefaultPage(vm.PID(1), tc.input, 0)
		if page.VAddr != tc.expected {
			t.Errorf("For input 0x%x, expected aligned VAddr 0x%x, got 0x%x", 
				tc.input, tc.expected, page.VAddr)
		}
	}
}