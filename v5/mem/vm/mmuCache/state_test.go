package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmuCache/internal"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Errorf("ValidateState(State{}) = %v, want nil", err)
	}
}

func buildTestCacheComp(name string, spec Spec) *Comp {
	modelComp := modeling.NewBuilder[Spec, State]().
		WithSpec(spec).
		Build(name)
	c := &Comp{
		Component: modelComp,
		state:     mmuCacheStateEnable,
	}
	c.table = make([]internal.Set, spec.NumLevels)
	for i := 0; i < spec.NumLevels; i++ {
		c.table[i] = internal.NewSet(spec.NumBlocks)
	}
	return c
}

func verifyCacheState(t *testing.T, s State) {
	t.Helper()

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
}

func verifyCacheRestore(t *testing.T, c *Comp) {
	t.Helper()

	if c.state != mmuCacheStateEnable {
		t.Errorf("restored state = %q, want %q", c.state, mmuCacheStateEnable)
	}
	if len(c.table) != 2 {
		t.Fatalf("restored table len = %d, want 2", len(c.table))
	}
	if c.inflightFlushReq == nil {
		t.Fatal("restored inflightFlushReq is nil")
	}
	if c.inflightFlushReq.ID != "flush-123" {
		t.Errorf("restored inflightFlushReq.ID = %q, want %q", c.inflightFlushReq.ID, "flush-123")
	}

	wayID, found := c.table[0].Lookup(vm.PID(1), 0x100)
	if !found {
		t.Error("table[0] lookup for PID=1,seg=0x100 not found")
	}
	if wayID != 0 {
		t.Errorf("table[0] wayID = %d, want 0", wayID)
	}

	wayID, found = c.table[1].Lookup(vm.PID(2), 0x200)
	if !found {
		t.Error("table[1] lookup for PID=2,seg=0x200 not found")
	}
	if wayID != 1 {
		t.Errorf("table[1] wayID = %d, want 1", wayID)
	}
}

func TestGetStateAndSetState(t *testing.T) {
	spec := Spec{
		NumBlocks: 2, NumLevels: 2, PageSize: 4096,
		Log2PageSize: 12, NumReqPerCycle: 4, LatencyPerLevel: 100,
	}

	c := buildTestCacheComp("TestMMUCache", spec)
	c.table[0].Update(0, vm.PID(1), 0x100)
	c.table[0].Visit(0)
	c.table[1].Update(1, vm.PID(2), 0x200)
	c.table[1].Visit(1)
	c.inflightFlushReq = &FlushReq{
		MsgMeta: sim.MsgMeta{
			ID: "flush-123", Src: sim.RemotePort("ctrl.port"),
		},
	}

	s := c.GetState()
	verifyCacheState(t, s)

	c2 := &Comp{
		Component: modeling.NewBuilder[Spec, State]().
			WithSpec(spec).
			Build("TestMMUCache2"),
	}
	c2.SetState(s)
	verifyCacheRestore(t, c2)
}

func TestGetStateNoInflightFlush(t *testing.T) {
	spec := Spec{
		NumBlocks:       1,
		NumLevels:       1,
		PageSize:        4096,
		Log2PageSize:    12,
		NumReqPerCycle:  1,
		LatencyPerLevel: 50,
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithSpec(spec).
		Build("TestNoFlush")

	c := &Comp{
		Component:        modelComp,
		state:            mmuCacheStatePause,
		inflightFlushReq: nil,
	}

	c.table = make([]internal.Set, spec.NumLevels)
	for i := 0; i < spec.NumLevels; i++ {
		c.table[i] = internal.NewSet(spec.NumBlocks)
	}

	s := c.GetState()

	if s.InflightFlushReqActive {
		t.Error("InflightFlushReqActive = true, want false")
	}
	if s.CurrentState != mmuCacheStatePause {
		t.Errorf("CurrentState = %q, want %q", s.CurrentState, mmuCacheStatePause)
	}

	// Restore
	c2 := &Comp{
		Component: modeling.NewBuilder[Spec, State]().
			WithSpec(spec).
			Build("TestNoFlush2"),
	}
	c2.SetState(s)

	if c2.inflightFlushReq != nil {
		t.Error("restored inflightFlushReq should be nil")
	}
	if c2.state != mmuCacheStatePause {
		t.Errorf("restored state = %q, want %q", c2.state, mmuCacheStatePause)
	}
}
