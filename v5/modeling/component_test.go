package modeling_test

import (
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
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

func TestComponentGetSpec(t *testing.T) {
	engine := sim.NewSerialEngine()
	spec := TestSpec{
		Frequency: 1.0,
		BufferLen: 4,
		Name:      "test",
		Enabled:   true,
	}

	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithSpec(spec).
		Build("TestComp")

	got := comp.GetSpec()
	if got != spec {
		t.Errorf("GetSpec() = %v, want %v", got, spec)
	}
}

func TestComponentGetState(t *testing.T) {
	engine := sim.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("TestComp")

	state := comp.GetState()
	if state.Counter != 0 {
		t.Errorf("GetState().Counter = %d, want 0", state.Counter)
	}
}

func TestComponentSetState(t *testing.T) {
	engine := sim.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("TestComp")

	newState := TestState{
		Counter:    42,
		Values:     []int{1, 2, 3},
		LastStatus: "running",
		Tags:       []string{"a", "b"},
	}
	comp.SetState(newState)

	got := comp.GetState()
	if got.Counter != 42 {
		t.Errorf("GetState().Counter = %d, want 42", got.Counter)
	}
	if got.LastStatus != "running" {
		t.Errorf("GetState().LastStatus = %q, want %q", got.LastStatus, "running")
	}
	if len(got.Values) != 3 {
		t.Errorf("GetState().Values len = %d, want 3", len(got.Values))
	}
}

func TestComponentSpecImmutableAfterCreation(t *testing.T) {
	engine := sim.NewSerialEngine()
	spec := TestSpec{
		Frequency: 2.0,
		BufferLen: 8,
		Name:      "original",
		Enabled:   true,
	}

	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithSpec(spec).
		Build("TestComp")

	// Modify the original; shouldn't affect the component's spec since it's
	// a value copy.
	spec.Name = "modified"

	got := comp.GetSpec()
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
	engine := sim.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
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
	engine := sim.NewSerialEngine()
	spec := TestSpec{Frequency: 5.0, BufferLen: 2, Name: "b", Enabled: true}

	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithSpec(spec).
		Build("BuilderComp")

	if comp.GetSpec() != spec {
		t.Errorf("builder spec = %v, want %v", comp.GetSpec(), spec)
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
