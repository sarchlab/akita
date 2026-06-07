package modeling

import (
	"encoding/json"
	"strings"
	"testing"
)

// hidden has only unexported fields and no MarshalJSON: it serializes as {} and
// silently loses its state — the lruset.Set class of bug.
type hidden struct {
	values []int
	index  map[string]int
}

// customJSON has only unexported fields but round-trips via MarshalJSON, so it
// is trusted.
type customJSON struct {
	values []int
}

func (c customJSON) MarshalJSON() ([]byte, error)  { return json.Marshal(c.values) }
func (c *customJSON) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &c.values) }

type stateWithHidden struct {
	Count  int    `json:"count"`
	Lookup hidden `json:"lookup"`
}

type stateWithCustom struct {
	Count int        `json:"count"`
	LRU   customJSON `json:"lru"`
}

type normalState struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

func TestValidateState_RejectsEmptyObjectStruct(t *testing.T) {
	// Populated here only so the fields count as used; the validator inspects the
	// type (it marshals a zero value), so the data-loss verdict is unchanged.
	err := ValidateState(hidden{values: []int{1}, index: map[string]int{"a": 1}})
	if err == nil || !strings.Contains(err.Error(), "serializes as {}") {
		t.Fatalf("expected data-loss error, got %v", err)
	}
}

func TestValidateState_RejectsNestedEmptyObjectStruct(t *testing.T) {
	err := ValidateState(stateWithHidden{})
	if err == nil || !strings.Contains(err.Error(), "serializes as {}") {
		t.Fatalf("expected data-loss error for nested field, got %v", err)
	}
}

func TestValidateState_AllowsCustomJSONMarshaler(t *testing.T) {
	if err := ValidateState(stateWithCustom{}); err != nil {
		t.Fatalf("nested custom-JSON type should pass: %v", err)
	}
	if err := ValidateState(customJSON{}); err != nil {
		t.Fatalf("top-level custom-JSON state should pass: %v", err)
	}
}

func TestValidateState_AllowsNormalAndEmptyStructs(t *testing.T) {
	if err := ValidateState(normalState{}); err != nil {
		t.Fatalf("normal state should pass: %v", err)
	}
	if err := ValidateState(None{}); err != nil {
		t.Fatalf("None (zero-field struct) should pass: %v", err)
	}
}
