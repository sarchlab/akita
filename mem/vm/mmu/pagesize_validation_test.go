package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// TestPageSizeValidation tests that the MMU validates page table page size consistency
func TestPageSizeValidation(t *testing.T) {
	sim := akitasim.MakeBuilder().Build()

	// Test case 1: Matching page sizes should work
	pageTable := vm.NewPageTable(12) // 4KB pages
	matchingSpec := DefaultSpec()
	matchingSpec.Log2PageSize = 12 // 4KB pages

	builder := modeling.MakeBuilder[*Comp](matchingSpec).
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable})

	// This should not panic
	mmu := builder.Build("MatchingPageSizes")
	ports := makePorts(sim, "", 4096)
	mmu.BindPort("Top", ports.Top)
	mmu.BindPort("Control", ports.Control)
	if mmu == nil {
		t.Error("MMU creation should succeed with matching page sizes")
	}

	// Test case 2: Mismatched page sizes should panic
	pageTable2 := vm.NewPageTable(12) // 4KB pages
	mismatchedSpec := DefaultSpec()
	mismatchedSpec.Log2PageSize = 16 // 64KB pages
	builder2 := modeling.MakeBuilder[*Comp](mismatchedSpec).
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable2})

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

	bad := builder2.Build("MismatchedPageSizes")
	badPorts := makePorts(sim, "", 4096)
	bad.BindPort("Top", badPorts.Top)
	bad.BindPort("Control", badPorts.Control)
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}
}

// TestPageTableRequired tests that Build panics when no page table is given.
func TestPageTableRequired(t *testing.T) {
	sim := akitasim.MakeBuilder().Build()

	defer func() {
		expectedMessage := "mmu: Resources.PageTable is required"
		if r := recover(); r != expectedMessage {
			t.Errorf("Expected panic with message '%s', got '%v'",
				expectedMessage, r)
		}
	}()

	builtComponent := modeling.MakeBuilder[*Comp](DefaultSpec()).
		WithSimulation(sim).
		Build("NoPageTable")
	builtComponentPorts := makePorts(sim, "NoPageTable", 4096)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}
}
