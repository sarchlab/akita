// Package modeling provides a generic component framework for Akita simulations.
//
// It defines Component[S, T, R], a generic component parameterized by a Spec
// type (immutable configuration), a State type (mutable runtime data), and a
// Resources type (references to external objects). A Spec holds only scalar
// fields. A State may also hold slices, arrays, maps, and simple nested
// structs. Both must be JSON-serializable and must not contain pointers,
// interfaces, or functions; those belong in Resources.
package modeling

// Spec is a constraint for component specifications.
//
// Specs must be plain structs with only scalar fields: bool, int, int8, int16,
// int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string,
// and named types based on them (such as timing.Freq or an enum-like string
// type). No slices, arrays, maps, nested structs, pointers, interfaces, or
// functions. A value a component derives in Build that needs a container
// belongs in State; a reference to an external object belongs in Resources.
//
// Go does not support a struct constraint, so this is typed as `any`.
// Use [ValidateSpec] at runtime to verify that a value conforms to these rules.
type Spec = any
