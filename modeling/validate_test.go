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

// marshalOnly customizes the save direction but not the load direction: a
// checkpoint saves its custom payload and then silently restores zero values,
// because the default decoder cannot set the unexported field.
type marshalOnly struct {
	values []int
}

func (m marshalOnly) MarshalJSON() ([]byte, error) { return json.Marshal(m.values) }

type stateWithMarshalOnly struct {
	Count int         `json:"count"`
	LRU   marshalOnly `json:"lru"`
}

func TestValidateState_RejectsMarshalerWithoutUnmarshaler(t *testing.T) {
	err := ValidateState(marshalOnly{})
	if err == nil || !strings.Contains(err.Error(), "no UnmarshalJSON") {
		t.Fatalf("expected missing-UnmarshalJSON error, got %v", err)
	}

	err = ValidateState(stateWithMarshalOnly{})
	if err == nil || !strings.Contains(err.Error(), "no UnmarshalJSON") {
		t.Fatalf("expected missing-UnmarshalJSON error for nested field, got %v",
			err)
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

// nestedCollectionState exercises element types that are themselves collections
// — all JSON-serializable, so all valid. map[K][]V is the MemAccessAgent case
// that regressed CI when element validation was not recursive.
type nestedCollectionState struct {
	MapOfSlices  map[uint64][]uint32      `json:"map_of_slices"`
	SliceOfSlice [][]int                  `json:"slice_of_slice"`
	Array        [4]byte                  `json:"array"`
	MapOfStructs map[string][]normalState `json:"map_of_structs"`
}

func TestValidateState_AllowsNestedCollections(t *testing.T) {
	if err := ValidateState(nestedCollectionState{}); err != nil {
		t.Fatalf("nested collections should pass: %v", err)
	}
}

// badNestedState hides a pointer inside a collection; the recursion must still
// reject it.
type badNestedState struct {
	MapOfPtrs map[string]*normalState `json:"map_of_ptrs"`
}

func TestValidateState_RejectsPointerInNestedCollection(t *testing.T) {
	if err := ValidateState(badNestedState{}); err == nil {
		t.Fatalf("a pointer nested in a map value should still be rejected")
	}
}

// dupJSONSpec's N is untagged, so its JSON name is the field name "N",
// colliding with B's explicit tag: encoding/json keeps only the tagged B.
// go vet only catches two identical explicit tags, not this collision.
type dupJSONSpec struct {
	N int
	B int `json:"N"`
}

type dupJSONNested struct {
	X int
	B int `json:"X"`
}

type stateWithDupJSON struct {
	Inner dupJSONNested `json:"inner"`
}

// distinctJSONState reuses a JSON name only on fields encoding/json ignores:
// an unexported field and one tagged "-".
type distinctJSONState struct {
	X       int `json:"x"`
	Skipped int `json:"-"`
	x       int
}

type embeddedA struct{ X int }
type embeddedB struct{ X int }

// stateWithEmbeddedCollision embeds two structs that both contribute X;
// encoding/json flattens them and drops both. Count keeps the struct from
// serializing as {}, which a separate check already rejects.
type stateWithEmbeddedCollision struct {
	embeddedA
	embeddedB
	Count int
}

// stateWithShadowedField has an outer X that hides the embedded one, which
// encoding/json then never encodes.
type stateWithShadowedField struct {
	embeddedA
	X int
}

type header struct{ Seq int }

// stateWithUnnamedEmbeddedTag embeds header with a tag that sets only an
// option. encoding/json still flattens it, so its JSON names are "Seq" and
// the explicit "Header" field, which do not collide.
type stateWithUnnamedEmbeddedTag struct {
	header `json:",omitempty"`
	Other  int `json:"Header"`
}

func TestValidateSpec_RejectsDuplicateJSONNames(t *testing.T) {
	err := ValidateSpec(dupJSONSpec{})
	if err == nil || !strings.Contains(err.Error(), `share JSON name "N"`) {
		t.Fatalf("expected duplicate JSON name error, got %v", err)
	}
}

func TestValidateState_RejectsDuplicateJSONNames(t *testing.T) {
	for name, state := range map[string]any{
		"nested struct":       stateWithDupJSON{},
		"embedded collision":  stateWithEmbeddedCollision{},
		"shadowed by outer X": stateWithShadowedField{},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateState(state)
			if err == nil || !strings.Contains(err.Error(), `share JSON name "X"`) {
				t.Fatalf("expected duplicate JSON name error, got %v", err)
			}
		})
	}
}

func TestValidateState_AllowsNamesIgnoredByJSON(t *testing.T) {
	for name, state := range map[string]any{
		"unexported and dash-tagged": distinctJSONState{x: 1},
		"embedded with option tag":   stateWithUnnamedEmbeddedTag{},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateState(state); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSpec_RejectsNonScalarDashField(t *testing.T) {
	type spec struct {
		N    int   `json:"n"`
		List []int `json:"-"`
	}

	if err := ValidateSpec(spec{}); err == nil {
		t.Fatal("expected a json:\"-\" slice field to be rejected in a Spec")
	}
}

func TestValidateState_AllowsDashFieldRebuiltBySetup(t *testing.T) {
	type state struct {
		N    int    `json:"n"`
		Hook func() `json:"-"`
	}

	if err := ValidateState(state{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
