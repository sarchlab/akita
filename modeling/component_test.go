package modeling_test

import (
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// --- Test Spec and State types ---

type TestSpec struct {
	Frequency float64 `json:"frequency"`
	BufferLen int     `json:"buffer_len"`
	Name      string  `json:"name"`
	Enabled   bool    `json:"enabled"`
}

type TestState struct {
	Counter    int      `json:"counter"`
	Values     []int    `json:"values"`
	LastStatus string   `json:"last_status"`
	Tags       []string `json:"tags"`
}

// --- Component tests ---

func TestComponentSpec(t *testing.T) {
	engine := timing.NewSerialEngine()
	spec := TestSpec{
		Frequency: 1.0,
		BufferLen: 4,
		Name:      "test",
		Enabled:   true,
	}

	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(spec).
		Build("TestComp")

	got := comp.Spec
	if got != spec {
		t.Errorf("Spec() = %v, want %v", got, spec)
	}
}

func TestComponentState(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	state := comp.State
	if state.Counter != 0 {
		t.Errorf("State().Counter = %d, want 0", state.Counter)
	}
}

func TestComponentStateAssignment(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	newState := TestState{
		Counter:    42,
		Values:     []int{1, 2, 3},
		LastStatus: "running",
		Tags:       []string{"a", "b"},
	}
	comp.State = newState

	got := comp.State
	if got.Counter != 42 {
		t.Errorf("State().Counter = %d, want 42", got.Counter)
	}
	if got.LastStatus != "running" {
		t.Errorf("State().LastStatus = %q, want %q", got.LastStatus, "running")
	}
	if len(got.Values) != 3 {
		t.Errorf("State().Values len = %d, want 3", len(got.Values))
	}
}

func TestComponentSpecImmutableAfterCreation(t *testing.T) {
	engine := timing.NewSerialEngine()
	spec := TestSpec{
		Frequency: 2.0,
		BufferLen: 8,
		Name:      "original",
		Enabled:   true,
	}

	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(spec).
		Build("TestComp")

	// Modify the original; shouldn't affect the component's spec since it's
	// a value copy.
	spec.Name = "modified"

	got := comp.Spec
	if got.Name != "original" {
		t.Errorf("spec was mutated: got %q, want %q", got.Name, "original")
	}
}

// --- JSON serialization tests ---

func TestSpecJSONSerialization(t *testing.T) {
	spec := TestSpec{
		Frequency: 3.5,
		BufferLen: 16,
		Name:      "json-test",
		Enabled:   false,
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal(spec) error: %v", err)
	}

	var decoded TestSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded != spec {
		t.Errorf("round-trip mismatch: got %v, want %v", decoded, spec)
	}
}

func TestStateJSONSerialization(t *testing.T) {
	state := TestState{
		Counter:    99,
		Values:     []int{10, 20, 30},
		LastStatus: "idle",
		Tags:       []string{"x"},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(state) error: %v", err)
	}

	var decoded TestState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded.Counter != state.Counter {
		t.Errorf("Counter = %d, want %d", decoded.Counter, state.Counter)
	}
	if decoded.LastStatus != state.LastStatus {
		t.Errorf("LastStatus = %q, want %q", decoded.LastStatus, state.LastStatus)
	}
	if len(decoded.Values) != len(state.Values) {
		t.Errorf("Values len = %d, want %d", len(decoded.Values), len(state.Values))
	}
}

// --- Middleware integration test ---

type countMiddleware struct {
	count *int
}

func (m *countMiddleware) Tick() bool {
	*m.count++
	return *m.count <= 3
}

func TestComponentMiddlewareTick(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	count := 0
	comp.AddMiddleware(&countMiddleware{count: &count})

	progress := comp.Tick()
	if !progress {
		t.Error("first Tick() should return true")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	comp.Tick()
	comp.Tick()

	progress = comp.Tick()
	if progress {
		t.Error("fourth Tick() should return false")
	}
	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}
}

// --- Builder tests ---

func TestBuilderWithSpec(t *testing.T) {
	engine := timing.NewSerialEngine()
	spec := TestSpec{Frequency: 5.0, BufferLen: 2, Name: "b", Enabled: true}

	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(spec).
		Build("BuilderComp")

	if comp.Spec != spec {
		t.Errorf("builder spec = %v, want %v", comp.Spec, spec)
	}

	if comp.Name() != "BuilderComp" {
		t.Errorf("name = %q, want %q", comp.Name(), "BuilderComp")
	}
}

// --- ValidateSpec tests ---

func TestValidateSpecValid(t *testing.T) {
	tests := []struct {
		name string
		v    any
	}{
		{"primitives only", TestSpec{Frequency: 1, BufferLen: 4, Name: "x", Enabled: true}},
		{"empty struct", struct{}{}},
		{"with slices", struct {
			Ids    []int
			Names  []string
			Floats []float64
		}{Ids: []int{1}, Names: []string{"a"}, Floats: []float64{1.0}}},
		{"with map", struct {
			Labels map[string]string
			Counts map[string]int
		}{Labels: map[string]string{"a": "b"}, Counts: map[string]int{"x": 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := modeling.ValidateSpec(tt.v); err != nil {
				t.Errorf("ValidateSpec() error: %v", err)
			}
		})
	}
}

func TestValidateSpecInvalid(t *testing.T) {
	type withPointer struct {
		P *int
	}

	type withInterface struct {
		I interface{}
	}

	type withFunc struct {
		F func()
	}

	type withChan struct {
		C chan int
	}

	type withNestedStruct struct {
		Inner struct {
			X int
		}
	}

	tests := []struct {
		name string
		v    any
	}{
		{"pointer field", withPointer{}},
		{"interface field", withInterface{}},
		{"func field", withFunc{}},
		{"chan field", withChan{}},
		{"nested struct", withNestedStruct{}},
		{"not a struct", 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := modeling.ValidateSpec(tt.v); err == nil {
				t.Error("ValidateSpec() expected error, got nil")
			}
		})
	}
}

// --- ValidateState tests ---

func TestValidateStateValid(t *testing.T) {
	type Inner struct {
		X int
		Y string
	}

	tests := []struct {
		name string
		v    any
	}{
		{"primitives only", TestState{Counter: 1, LastStatus: "ok"}},
		{"with nested struct", struct {
			Data Inner
		}{Data: Inner{X: 1, Y: "y"}}},
		{"with slice of structs", struct {
			Items []Inner
		}{Items: []Inner{{X: 1, Y: "a"}}}},
		{"with map of structs", struct {
			Lookup map[string]Inner
		}{Lookup: map[string]Inner{"k": {X: 2, Y: "b"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := modeling.ValidateState(tt.v); err != nil {
				t.Errorf("ValidateState() error: %v", err)
			}
		})
	}
}

func TestValidateStateInvalid(t *testing.T) {
	type withPointer struct {
		P *int
	}

	type withFunc struct {
		F func()
	}

	tests := []struct {
		name string
		v    any
	}{
		{"pointer field", withPointer{}},
		{"func field", withFunc{}},
		{"not a struct", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := modeling.ValidateState(tt.v); err == nil {
				t.Error("ValidateState() expected error, got nil")
			}
		})
	}
}

// --- Single-state mutation tests ---

func TestStateReturnsState(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = TestState{Counter: 10, LastStatus: "init"}

	got := comp.State
	if got.Counter != 10 {
		t.Errorf("State().Counter = %d, want 10", got.Counter)
	}
	if got.LastStatus != "init" {
		t.Errorf("State().LastStatus = %q, want %q", got.LastStatus, "init")
	}
}

func TestStatePtrReturnsWritablePointer(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = TestState{Counter: 5}

	state := &comp.State
	state.Counter = 99
	state.LastStatus = "modified"

	got := comp.State
	if got.Counter != 99 {
		t.Errorf("State().Counter = %d, want 99", got.Counter)
	}
	if got.LastStatus != "modified" {
		t.Errorf("State().LastStatus = %q, want %q", got.LastStatus, "modified")
	}
}

// stateModifyMiddleware modifies component state during Tick via StatePtr.
type stateModifyMiddleware struct {
	comp *modeling.Component[TestSpec, TestState, modeling.None]
}

func (m *stateModifyMiddleware) Tick() bool {
	state := &m.comp.State
	state.Counter += 10
	state.LastStatus = "ticked"

	return true
}

func TestTickMutatesStateInPlace(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = TestState{Counter: 5, LastStatus: "init"}
	comp.AddMiddleware(&stateModifyMiddleware{comp: comp})

	comp.Tick()

	got := comp.State
	if got.Counter != 15 {
		t.Errorf("after Tick, State().Counter = %d, want 15", got.Counter)
	}
	if got.LastStatus != "ticked" {
		t.Errorf("after Tick, State().LastStatus = %q, want %q", got.LastStatus, "ticked")
	}
}

type stateCheckMiddleware struct {
	comp           *modeling.Component[TestSpec, TestState, modeling.None]
	beforeMutation int
	afterMutation  int
}

func (m *stateCheckMiddleware) Tick() bool {
	m.beforeMutation = m.comp.State.Counter

	state := &m.comp.State
	state.Counter = 999
	m.afterMutation = m.comp.State.Counter

	return true
}

func TestStatePtrMutationAffectsStateDuringTick(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = TestState{Counter: 42}

	mw := &stateCheckMiddleware{comp: comp}
	comp.AddMiddleware(mw)

	comp.Tick()

	if mw.beforeMutation != 42 {
		t.Errorf("state before mutation = %d, want 42", mw.beforeMutation)
	}
	if mw.afterMutation != 999 {
		t.Errorf("state after mutation = %d, want 999", mw.afterMutation)
	}
	if comp.State.Counter != 999 {
		t.Errorf("after Tick, State().Counter = %d, want 999", comp.State.Counter)
	}
}

func TestInPlaceSliceUpdate(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = TestState{
		Counter: 1,
		Values:  []int{10, 20, 30},
		Tags:    []string{"a", "b"},
	}

	state := &comp.State
	state.Tags[0] = "modified"

	got := comp.State
	if got.Tags[0] != "modified" {
		t.Errorf("state Tags[0] = %q, want %q", got.Tags[0], "modified")
	}
}

func TestInPlaceMapUpdate(t *testing.T) {
	type MapState struct {
		Counts map[string]int `json:"counts"`
	}

	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, MapState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	comp.State = MapState{Counts: map[string]int{"a": 1, "b": 2}}

	state := &comp.State
	state.Counts["c"] = 3
	state.Counts["a"] = 100

	got := comp.State
	if len(got.Counts) != 3 {
		t.Errorf("state Counts len = %d, want 3", len(got.Counts))
	}
	if got.Counts["a"] != 100 {
		t.Errorf("state Counts[a] = %d, want 100", got.Counts["a"])
	}
}

func TestStateAssignmentReplacesState(t *testing.T) {
	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("TestComp")

	state := TestState{Counter: 77, Values: []int{1, 2}, LastStatus: "synced"}
	comp.State = state

	if comp.State.Counter != 77 {
		t.Errorf("State().Counter = %d, want 77", comp.State.Counter)
	}
	if (&comp.State).Counter != 77 {
		t.Errorf("StatePtr().Counter = %d, want 77", (&comp.State).Counter)
	}

	(&comp.State).Counter = 88
	if comp.State.Counter != 88 {
		t.Errorf("State().Counter = %d, want 88",
			comp.State.Counter)
	}
}
