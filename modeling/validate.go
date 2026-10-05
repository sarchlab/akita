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

// ValidateSpec checks that the given value is a struct whose fields, including
// fields tagged `json:"-"`, are scalars (bool, int*, uint*, float*, string, and
// named types based on them) or slices and arrays of scalars, such as a list
// of remote ports whose length depends on the configuration. Maps, nested
// structs, slices of containers, pointers, interfaces, channels, and functions
// are not allowed: a Spec is flat configuration, and anything a component
// derives or references belongs in State or Resources.
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
