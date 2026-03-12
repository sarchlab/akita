package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("ValidateState(State{}) failed: %v", err)
	}
}

func buildTestMMU(engine sim.Engine, name string) *Comp {
	return MakeBuilder().
		WithEngine(engine).
		WithAutoPageAllocation(true).
		WithTopPort(sim.NewPort(nil, 4096, 4096, name+".ToTop")).
		WithMigrationPort(sim.NewPort(nil, 1, 1, name+".MigrationPort")).
		WithMigrationServiceProvider(sim.RemotePort("MigrationService")).
		Build(name)
}

func makeTestState(reqID string) State {
	return State{
		WalkingTranslations: []transactionState{
			{
				ReqID:    reqID,
				ReqSrc:   sim.RemotePort("Agent"),
				ReqDst:   sim.RemotePort("TestMMU.ToTop"),
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
		MigrationQueue: []transactionState{
			{
				ReqID:    reqID,
				ReqSrc:   sim.RemotePort("Agent"),
				ReqDst:   sim.RemotePort("TestMMU.ToTop"),
				PID:      1,
				VAddr:    0x3000,
				DeviceID: 3,
				Page: vm.Page{
					PID: 1, VAddr: 0x3000, PAddr: 0x4000,
					PageSize: 4096, Valid: true, DeviceID: 3,
				},
				CycleLeft: 2,
			},
		},
		IsDoingMigration: true,
		NextPhysicalPage: 0x8000,
		PageAccessedByDeviceID: []devicePageAccess{
			{PageVAddr: 0x1000, DeviceIDs: []uint64{1, 2}},
			{PageVAddr: 0x3000, DeviceIDs: []uint64{3}},
		},
	}
}

func verifyState(t *testing.T, got State, reqID string) {
	t.Helper()

	if len(got.WalkingTranslations) != 1 {
		t.Fatalf("expected 1 walking translation, got %d",
			len(got.WalkingTranslations))
	}
	if got.WalkingTranslations[0].ReqID != reqID {
		t.Errorf("expected req ID %s, got %s",
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
	if len(got.MigrationQueue) != 1 {
		t.Fatalf("expected 1 migration queue entry, got %d",
			len(got.MigrationQueue))
	}
	if !got.IsDoingMigration {
		t.Error("expected IsDoingMigration to be true")
	}
	if got.NextPhysicalPage != 0x8000 {
		t.Errorf("expected NextPhysicalPage 0x8000, got 0x%x",
			got.NextPhysicalPage)
	}
	if len(got.PageAccessedByDeviceID) != 2 {
		t.Errorf("expected 2 device page access entries, got %d",
			len(got.PageAccessedByDeviceID))
	}
}

func TestGetStateAndSetState(t *testing.T) {
	engine := sim.NewSerialEngine()
	mmu := buildTestMMU(engine, "TestMMU")

	reqID := sim.GetIDGenerator().Generate()
	state := makeTestState(reqID)

	mmu.SetState(state)
	got := mmu.GetState()

	verifyState(t, got, reqID)
}
