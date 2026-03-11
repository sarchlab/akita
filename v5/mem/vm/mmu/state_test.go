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

func populateMMURuntimeState(mmu *Comp, req *vm.TranslationReq) {
	mmu.walkingTranslations = []transaction{
		{
			req: req,
			page: vm.Page{
				PID: 1, VAddr: 0x1000, PAddr: 0x2000,
				PageSize: 4096, Valid: true, DeviceID: 2, Unified: true,
			},
			cycleLeft: 5,
		},
	}
	mmu.migrationQueue = []transaction{
		{
			req: req,
			page: vm.Page{
				PID: 1, VAddr: 0x3000, PAddr: 0x4000,
				PageSize: 4096, Valid: true, DeviceID: 3,
			},
			cycleLeft: 2,
		},
	}
	mmu.isDoingMigration = true
	mmu.nextPhysicalPage = 0x8000
	mmu.PageAccessedByDeviceID[0x1000] = []uint64{1, 2}
	mmu.PageAccessedByDeviceID[0x3000] = []uint64{3}
}

func verifyMMUState(t *testing.T, state State, reqID string) {
	t.Helper()

	if len(state.WalkingTranslations) != 1 {
		t.Fatalf("expected 1 walking translation, got %d", len(state.WalkingTranslations))
	}
	if state.WalkingTranslations[0].ReqID != reqID {
		t.Errorf("expected req ID %s, got %s", reqID, state.WalkingTranslations[0].ReqID)
	}
	if state.WalkingTranslations[0].CycleLeft != 5 {
		t.Errorf("expected cycle_left 5, got %d", state.WalkingTranslations[0].CycleLeft)
	}
	if state.WalkingTranslations[0].Page.PAddr != 0x2000 {
		t.Errorf("expected page PAddr 0x2000, got 0x%x", state.WalkingTranslations[0].Page.PAddr)
	}
	if len(state.MigrationQueue) != 1 {
		t.Fatalf("expected 1 migration queue entry, got %d", len(state.MigrationQueue))
	}
	if !state.IsDoingMigration {
		t.Error("expected IsDoingMigration to be true")
	}
	if state.NextPhysicalPage != 0x8000 {
		t.Errorf("expected NextPhysicalPage 0x8000, got 0x%x", state.NextPhysicalPage)
	}
	if len(state.PageAccessedByDeviceID) != 2 {
		t.Errorf("expected 2 device page access entries, got %d", len(state.PageAccessedByDeviceID))
	}
}

func verifyMMURestore(t *testing.T, mmu *Comp, reqID string) {
	t.Helper()

	if len(mmu.walkingTranslations) != 1 {
		t.Fatalf("restored: expected 1 walking translation, got %d", len(mmu.walkingTranslations))
	}
	if mmu.walkingTranslations[0].req.ID != reqID {
		t.Errorf("restored: expected req ID %s, got %s", reqID, mmu.walkingTranslations[0].req.ID)
	}
	if mmu.walkingTranslations[0].cycleLeft != 5 {
		t.Errorf("restored: expected cycleLeft 5, got %d", mmu.walkingTranslations[0].cycleLeft)
	}
	if mmu.walkingTranslations[0].page.PAddr != 0x2000 {
		t.Errorf("restored: expected page PAddr 0x2000, got 0x%x", mmu.walkingTranslations[0].page.PAddr)
	}
	if mmu.walkingTranslations[0].req.VAddr != 0x1000 {
		t.Errorf("restored: expected req VAddr 0x1000, got 0x%x", mmu.walkingTranslations[0].req.VAddr)
	}
	if !mmu.isDoingMigration {
		t.Error("restored: expected isDoingMigration to be true")
	}
	if mmu.nextPhysicalPage != 0x8000 {
		t.Errorf("restored: expected nextPhysicalPage 0x8000, got 0x%x", mmu.nextPhysicalPage)
	}
	if len(mmu.PageAccessedByDeviceID) != 2 {
		t.Errorf("restored: expected 2 device page access entries, got %d", len(mmu.PageAccessedByDeviceID))
	}
	if len(mmu.PageAccessedByDeviceID[0x1000]) != 2 {
		t.Errorf("restored: expected 2 device IDs for page 0x1000, got %d", len(mmu.PageAccessedByDeviceID[0x1000]))
	}
}

func TestGetStateAndSetState(t *testing.T) {
	engine := sim.NewSerialEngine()
	mmu := buildTestMMU(engine, "TestMMU")

	req := &vm.TranslationReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = sim.RemotePort("Agent")
	req.Dst = sim.RemotePort("MMU.ToTop")
	req.PID = 1
	req.VAddr = 0x1000
	req.DeviceID = 2
	req.TrafficClass = "vm.TranslationReq"
	populateMMURuntimeState(mmu, req)

	state := mmu.GetState()
	verifyMMUState(t, state, req.ID)

	mmu2 := buildTestMMU(engine, "TestMMU2")
	mmu2.SetState(state)
	verifyMMURestore(t, mmu2, req.ID)
}

func TestTransactionStateWithMigration(t *testing.T) {
	migrationReq := &vm.PageMigrationReqToDriver{
		MsgMeta: sim.MsgMeta{
			ID:  "mig-123",
			Src: sim.RemotePort("MMU.MigrationPort"),
			Dst: sim.RemotePort("MigrationService"),
		},
	}

	req := &vm.TranslationReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = sim.RemotePort("Agent")
	req.Dst = sim.RemotePort("MMU.ToTop")
	req.PID = 1
	req.VAddr = 0x1000
	req.DeviceID = 2
	req.TrafficClass = "vm.TranslationReq"

	trans := transaction{
		req: req,
		page: vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			PageSize: 4096,
			Valid:    true,
		},
		cycleLeft: 3,
		migration: migrationReq,
	}

	ts := transToState(trans)

	if !ts.HasMigration {
		t.Error("expected HasMigration to be true")
	}
	if ts.MigrationReqID != "mig-123" {
		t.Errorf("expected migration req ID mig-123, got %s", ts.MigrationReqID)
	}

	restored := stateToTrans(ts)
	if restored.migration == nil {
		t.Fatal("expected migration to be non-nil")
	}
	if restored.migration.ID != "mig-123" {
		t.Errorf("restored: expected migration ID mig-123, got %s", restored.migration.ID)
	}
}
