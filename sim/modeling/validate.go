package modeling

import (
	"fmt"

	"github.com/sarchlab/akita/v5/internal/valuecheck"
)

// MustBeCheckpointable checks a component's Spec and State so a mis-modeled
// component fails loudly at construction rather than silently producing a wrong
// resume. It panics — like the other builder misconfiguration guards — because a
// non-serializable Spec/State is a programming error, not a runtime condition.
func MustBeCheckpointable[S, T any](name string, spec S) {
	if err := ValidateSpec(spec); err != nil {
		panic(fmt.Sprintf(
			"modeling: component %q has a Spec that cannot be checkpointed: %v",
			name, err))
	}

	var zeroState T
	if err := ValidateState(zeroState); err != nil {
		panic(fmt.Sprintf(
			"modeling: component %q has a State that cannot be checkpointed: %v",
			name, err))
	}
}

// ValidateSpec checks that the given value is a struct with exported fields
// that are not tagged `json:"-"`. Fields must be scalars (bool, int*, uint*,
// float*, string, and named types based on them), scalar arrays/slices, or
// flat records or slices of flat records. Record fields must be exported scalars
// or scalar slices with explicit JSON names. Nested records, other nested
// containers, maps, pointers, interfaces, channels, and functions are
// not allowed. Derived lookup objects belong in middleware; shared live objects
// belong in Resources. Custom JSON codecs must
// implement both MarshalJSON and UnmarshalJSON and round-trip the given value.
// A field whose type defines IsZero() bool must not use json:",omitzero": its
// custom zero predicate could omit distinct configuration values from the hash.
func ValidateSpec(v any) error {
	return valuecheck.Spec(v)
}

// ValidateState checks that the given value is a struct containing only
// scalar fields, slices, arrays, maps with string or integer keys, and simple
// nested structs. Pointers, interfaces, channels, and functions are not
// allowed: a State refers to another component by name, not by pointer.
func ValidateState(v any) error {
	return valuecheck.State(v)
}
