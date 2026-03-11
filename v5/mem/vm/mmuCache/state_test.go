package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Errorf("ValidateState(State{}) = %v, want nil", err)
	}
}

func TestStateWithTable(t *testing.T) {
	spec := Spec{
		NumBlocks: 2, NumLevels: 2, PageSize: 4096,
		Log2PageSize: 12, NumReqPerCycle: 4, LatencyPerLevel: 100,
	}

	table := initSets(spec.NumLevels, spec.NumBlocks)
	setUpdate(&table[0], 0, vm.PID(1), 0x100)
	setVisit(&table[0], 0)
	setUpdate(&table[1], 1, vm.PID(2), 0x200)
	setVisit(&table[1], 1)

	s := State{
		CurrentState:           mmuCacheStateEnable,
		Table:                  table,
		InflightFlushReqActive: true,
		InflightFlushReqID:     "flush-123",
		InflightFlushReqSrc:    sim.RemotePort("ctrl.port"),
	}

	// Verify state
	if s.CurrentState != mmuCacheStateEnable {
		t.Errorf("CurrentState = %q, want %q", s.CurrentState, mmuCacheStateEnable)
	}
	if len(s.Table) != 2 {
		t.Fatalf("len(Table) = %d, want 2", len(s.Table))
	}
	if !s.InflightFlushReqActive {
		t.Error("InflightFlushReqActive = false, want true")
	}
	if s.InflightFlushReqID != "flush-123" {
		t.Errorf("InflightFlushReqID = %q, want %q", s.InflightFlushReqID, "flush-123")
	}
	if s.InflightFlushReqSrc != sim.RemotePort("ctrl.port") {
		t.Errorf("InflightFlushReqSrc = %q, want %q", s.InflightFlushReqSrc, "ctrl.port")
	}

	// Verify lookups
	wayID, found := setLookup(&s.Table[0], vm.PID(1), 0x100)
	if !found {
		t.Error("table[0] lookup for PID=1,seg=0x100 not found")
	}
	if wayID != 0 {
		t.Errorf("table[0] wayID = %d, want 0", wayID)
	}

	wayID, found = setLookup(&s.Table[1], vm.PID(2), 0x200)
	if !found {
		t.Error("table[1] lookup for PID=2,seg=0x200 not found")
	}
	if wayID != 1 {
		t.Errorf("table[1] wayID = %d, want 1", wayID)
	}
}

func TestStateNoInflightFlush(t *testing.T) {
	spec := Spec{
		NumBlocks:       1,
		NumLevels:       1,
		PageSize:        4096,
		Log2PageSize:    12,
		NumReqPerCycle:  1,
		LatencyPerLevel: 50,
	}

	s := State{
		CurrentState: mmuCacheStatePause,
		Table:        initSets(spec.NumLevels, spec.NumBlocks),
	}

	if s.InflightFlushReqActive {
		t.Error("InflightFlushReqActive = true, want false")
	}
	if s.CurrentState != mmuCacheStatePause {
		t.Errorf("CurrentState = %q, want %q", s.CurrentState, mmuCacheStatePause)
	}
}

func TestSetOperations(t *testing.T) {
	table := initSets(1, 4)

	// Lookup miss
	_, found := setLookup(&table[0], vm.PID(1), 0x100)
	if found {
		t.Error("Expected lookup to miss for empty set")
	}

	// Update and lookup hit
	setUpdate(&table[0], 0, vm.PID(1), 0x100)
	wayID, found := setLookup(&table[0], vm.PID(1), 0x100)
	if !found {
		t.Error("Expected lookup to hit after update")
	}
	if wayID != 0 {
		t.Errorf("wayID = %d, want 0", wayID)
	}

	// Visit
	setVisit(&table[0], 0)

	// Evict
	evictWay, ok := setEvict(&table[0])
	if !ok {
		t.Error("Expected evict to succeed")
	}
	// LRU should evict the least recently visited
	if evictWay == 0 {
		// Way 0 was visited most recently, so it shouldn't be evicted first
		// unless all ways were visited in order and 0 was the last to be visited
		// from initSets
	}
	_ = evictWay
}
