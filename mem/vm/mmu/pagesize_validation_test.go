package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
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
		WithSpec(matchingSpec)

	// This should not panic
	mmu := builder.Build("MatchingPageSizes")
	ports := makePorts("", 4096)
	mmu.BindPort("Top", ports.Top)
	mmu.BindPort("Control", ports.Control)
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
		WithSpec(mismatchedSpec)

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
	badPorts := makePorts("", 4096)
	bad.BindPort("Top", badPorts.Top)
	bad.BindPort("Control", badPorts.Control)
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}
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

	builtComponent := Definition.Builder().
		WithSimulation(sim).
		Build("NoPageTable")
	builtComponentPorts := makePorts("NoPageTable", 4096)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	if err := sim.Initialize(); err != nil {
		t.Fatal(err)
	}
}
