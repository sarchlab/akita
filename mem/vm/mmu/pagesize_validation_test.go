package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// TestPageSizeValidation tests that the MMU validates page table page size consistency
func TestPageSizeValidation(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Test case 1: Matching page sizes should work
	pageTable := vm.NewPageTable(12) // 4KB pages
	builder := MakeBuilder().
		WithEngine(engine).
		WithPageTable(pageTable).
		WithLog2PageSize(12) // 4KB pages

	// This should not panic
	mmu := builder.Build("MatchingPageSizes")
	if mmu == nil {
		t.Error("MMU creation should succeed with matching page sizes")
	}

	// Test case 2: Mismatched page sizes should panic
	pageTable2 := vm.NewPageTable(12) // 4KB pages
	builder2 := MakeBuilder().
		WithEngine(engine).
		WithPageTable(pageTable2).
		WithLog2PageSize(16) // 64KB pages

	// This should panic
	defer func() {
		if r := recover(); r != nil {
			expectedMessage := "page table page size does not match MMU page size"
			if r != expectedMessage {
				t.Errorf("Expected panic with message '%s', got '%v'", expectedMessage, r)
			}
		} else {
			t.Error("Expected panic for mismatched page sizes, but none occurred")
		}
	}()

	builder2.Build("MismatchedPageSizes") // Should panic
}