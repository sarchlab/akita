package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("ValidateState(State{}) failed: %v", err)
	}
}

func buildTestMMU(engine timing.Engine, name string) *Comp {
	spec := DefaultSpec()
	spec.AutoPageAllocation = true
	return MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		WithSpec(spec).
		Build(name)
}

func makeTestState(reqID uint64) State {
	return State{
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

func verifyState(t *testing.T, got State, reqID uint64) {
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
	engine := timing.NewSerialEngine()
	mmu := buildTestMMU(engine, "TestMMU")

	reqID := timing.GetIDGenerator().Generate()
	state := makeTestState(reqID)

	mmu.State = state
	got := mmu.State

	verifyState(t, got, reqID)
}
