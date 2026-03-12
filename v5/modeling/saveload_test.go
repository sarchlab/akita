package modeling_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	simengine "github.com/sarchlab/akita/v5/sim/engine"
)

func makeTestComponent(spec TestSpec, state TestState) *modeling.Component[TestSpec, TestState] {
	engine := simengine.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithSpec(spec).
		Build("SaveLoadComp")
	comp.SetState(state)

	return comp
}

func TestSaveStateWritesJSON(t *testing.T) {
	spec := TestSpec{Frequency: 2.5, BufferLen: 8, Name: "save-test", Enabled: true}
	state := TestState{Counter: 7, Values: []int{1, 2}, LastStatus: "active", Tags: []string{"t1"}}
	comp := makeTestComponent(spec, state)

	var buf bytes.Buffer
	if err := comp.SaveState(&buf); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Verify it's valid JSON with expected structure.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if _, ok := raw["spec"]; !ok {
		t.Error("JSON missing 'spec' key")
	}

	if _, ok := raw["state"]; !ok {
		t.Error("JSON missing 'state' key")
	}
}

func verifySpec(t *testing.T, got, want TestSpec) {
	t.Helper()

	if got.Frequency != want.Frequency {
		t.Errorf("spec.Frequency = %v, want %v", got.Frequency, want.Frequency)
	}

	if got.BufferLen != want.BufferLen {
		t.Errorf("spec.BufferLen = %v, want %v", got.BufferLen, want.BufferLen)
	}

	if got.Name != want.Name {
		t.Errorf("spec.Name = %q, want %q", got.Name, want.Name)
	}

	if got.Enabled != want.Enabled {
		t.Errorf("spec.Enabled = %v, want %v", got.Enabled, want.Enabled)
	}
}

func verifyState(t *testing.T, got, want TestState) {
	t.Helper()

	if got.Counter != want.Counter {
		t.Errorf("state.Counter = %d, want %d", got.Counter, want.Counter)
	}

	if got.LastStatus != want.LastStatus {
		t.Errorf("state.LastStatus = %q, want %q", got.LastStatus, want.LastStatus)
	}

	if len(got.Values) != len(want.Values) {
		t.Fatalf("state.Values len = %d, want %d", len(got.Values), len(want.Values))
	}

	for i, v := range got.Values {
		if v != want.Values[i] {
			t.Errorf("state.Values[%d] = %d, want %d", i, v, want.Values[i])
		}
	}

	if len(got.Tags) != len(want.Tags) {
		t.Fatalf("state.Tags len = %d, want %d", len(got.Tags), len(want.Tags))
	}

	for i, v := range got.Tags {
		if v != want.Tags[i] {
			t.Errorf("state.Tags[%d] = %q, want %q", i, v, want.Tags[i])
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	spec := TestSpec{Frequency: 3.0, BufferLen: 16, Name: "round-trip", Enabled: false}
	state := TestState{Counter: 42, Values: []int{10, 20, 30}, LastStatus: "idle", Tags: []string{"a", "b"}}
	comp := makeTestComponent(spec, state)

	// Save
	var buf bytes.Buffer
	if err := comp.SaveState(&buf); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Create a new component and load the state into it.
	engine := simengine.NewSerialEngine()
	comp2 := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("LoadedComp")

	if err := comp2.LoadState(&buf); err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}

	verifySpec(t, comp2.GetSpec(), spec)
	verifyState(t, comp2.GetState(), state)
}

func TestLoadStateWithZeroState(t *testing.T) {
	spec := TestSpec{Frequency: 1.0, BufferLen: 4, Name: "zero", Enabled: true}
	state := TestState{} // zero value
	comp := makeTestComponent(spec, state)

	var buf bytes.Buffer
	if err := comp.SaveState(&buf); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	engine := simengine.NewSerialEngine()
	comp2 := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("ZeroComp")

	if err := comp2.LoadState(&buf); err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}

	if comp2.GetSpec() != spec {
		t.Errorf("spec mismatch after loading zero state")
	}

	if comp2.GetState().Counter != 0 {
		t.Errorf("state.Counter = %d, want 0", comp2.GetState().Counter)
	}
}

func TestLoadStateInvalidJSON(t *testing.T) {
	engine := simengine.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("InvalidComp")

	r := bytes.NewReader([]byte("not valid json"))

	err := comp.LoadState(r)
	if err == nil {
		t.Error("LoadState() expected error for invalid JSON, got nil")
	}
}

// errWriter always returns an error on Write.
type errWriter struct{}

func (e *errWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestSaveStateWriteError(t *testing.T) {
	spec := TestSpec{Frequency: 1.0, BufferLen: 2, Name: "err", Enabled: false}
	state := TestState{Counter: 1}
	comp := makeTestComponent(spec, state)

	err := comp.SaveState(&errWriter{})
	if err == nil {
		t.Error("SaveState() expected error for writer failure, got nil")
	}
}

// errReader always returns an error on Read.
type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestLoadStateReadError(t *testing.T) {
	engine := simengine.NewSerialEngine()
	comp := modeling.NewBuilder[TestSpec, TestState]().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("ReadErrComp")

	err := comp.LoadState(&errReader{})
	if err == nil {
		t.Error("LoadState() expected error for reader failure, got nil")
	}
}
