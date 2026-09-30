package modeling_test

import (
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

// --- ValidateSpec tests ---

// testKind is an enum-like named string type, a valid Spec field type.
type testKind string

func TestValidateSpecValid(t *testing.T) {
	tests := []struct {
		name string
		v    any
	}{
		{"primitives only", TestSpec{Frequency: 1, BufferLen: 4, Name: "x", Enabled: true}},
		{"empty struct", struct{}{}},
		{"named scalar types", struct {
			Freq timing.Freq
			Kind testKind
		}{Freq: 1 * timing.GHz, Kind: "fast"}},
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

	type withSlice struct {
		IDs []int
	}

	type withArray struct {
		Lanes [4]int
	}

	type withMap struct {
		Labels map[string]string
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
		{"slice field", withSlice{}},
		{"array field", withArray{}},
		{"map field", withMap{}},
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
