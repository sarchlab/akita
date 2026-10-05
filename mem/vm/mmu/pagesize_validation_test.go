package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// TestPageSizeValidation tests that the MMU validates page table page size consistency
func TestPageSizeValidation(t *testing.T) {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	// Test case 1: Matching page sizes should work
	pageTable := vm.NewPageTable(12) // 4KB pages
	matchingSpec := Definition.DefaultSpec
	matchingSpec.Log2PageSize = 12 // 4KB pages

	builder := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable}).
		WithSpec(matchingSpec).
		WithPorts(makePorts("MatchingPageSizes", 4096))

	// This should not panic
	mmu := builder.Build("MatchingPageSizes")
	if mmu == nil {
		t.Error("MMU creation should succeed with matching page sizes")
	}

	// Test case 2: Mismatched page sizes should panic
	pageTable2 := vm.NewPageTable(12) // 4KB pages
	mismatchedSpec := Definition.DefaultSpec
	mismatchedSpec.Log2PageSize = 16 // 64KB pages
	builder2 := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable2}).
		WithSpec(mismatchedSpec).
		WithPorts(makePorts("MismatchedPageSizes", 4096))

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

// TestPageTableRequired tests that Build panics when no page table is given.
func TestPageTableRequired(t *testing.T) {
	sim := modeling.NewStandaloneSimulation(timing.NewSerialEngine())

	defer func() {
		expectedMessage := "mmu: Resources.PageTable is required"
		if r := recover(); r != expectedMessage {
			t.Errorf("Expected panic with message '%s', got '%v'",
				expectedMessage, r)
		}
	}()

	Definition.Builder().
		WithSimulation(sim).
		WithPorts(makePorts("NoPageTable", 4096)).
		Build("NoPageTable") // Should panic
}
