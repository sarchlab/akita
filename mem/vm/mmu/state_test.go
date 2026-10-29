package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("ValidateState(State{}) failed: %v", err)
	}
}

func buildTestMMU(sim *testsim.Simulation, name string) *Comp {
	spec := DefaultSpec()
	spec.AutoPageAllocation = true

	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{PageTable: vm.NewPageTable(spec.Log2PageSize)}).
		Build(name)
	compPorts := makePorts(sim, name, 4096)
	comp.BindPort("Top", compPorts.Top)
	comp.BindPort("Control", compPorts.Control)
	comp.BindPort("Notification", compPorts.Notification)

	return comp
}

func makeTestState(reqID uint64) state {
	return state{
		WalkingTranslations: []transactionState{
			{
				ReqID:    reqID,
				ReqSrc:   messaging.RemotePort("Agent"),
				ReqDst:   messaging.RemotePort("TestMMU.ToTop"),
				PID:      1,
				VAddr:    0x1000,
				DeviceID: 2,
				Page: vm.Page{
					PID: 1, VAddr: 0x1000, PAddr: 0x2000,
					PageSize: 4096, Valid: true, DeviceID: 2, Unified: true,
				},
				CycleLeft: 5,
			},
		},
		NextPhysicalPage: 0x8000,
	}
}

func verifyState(t *testing.T, got state, reqID uint64) {
	t.Helper()

	if len(got.WalkingTranslations) != 1 {
		t.Fatalf("expected 1 walking translation, got %d",
			len(got.WalkingTranslations))
	}
	if got.WalkingTranslations[0].ReqID != reqID {
		t.Errorf("expected req ID %d, got %d",
			reqID, got.WalkingTranslations[0].ReqID)
	}
	if got.WalkingTranslations[0].CycleLeft != 5 {
		t.Errorf("expected cycle_left 5, got %d",
			got.WalkingTranslations[0].CycleLeft)
	}
	if got.WalkingTranslations[0].Page.PAddr != 0x2000 {
		t.Errorf("expected page PAddr 0x2000, got 0x%x",
			got.WalkingTranslations[0].Page.PAddr)
	}
	if got.NextPhysicalPage != 0x8000 {
		t.Errorf("expected NextPhysicalPage 0x8000, got 0x%x",
			got.NextPhysicalPage)
	}
}

func TestStateAndStateAssignment(t *testing.T) {
	sim := akitasim.MakeBuilder().Build()

	mmu := buildTestMMU(sim, "TestMMU")

	reqID := sim.NewID()
	state := makeTestState(reqID)

	mmu.State = state
	got := mmu.State

	verifyState(t, got, reqID)
}
