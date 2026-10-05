# naming — Hierarchical Name Conventions

Package `naming` provides the naming conventions and utilities for Akita
simulation components. It defines the `Named` interface, a parser for
hierarchical, dot-separated names, and helpers for building and validating
names.

## The `Named` Interface

Any object that has a name implements `Named`:

```go
type Named interface {
    Name() string
}
```

## Name Structure

A name is a series of dot-separated tokens. Each token has an element name and
zero or more square-bracket indices, which may be multi-dimensional:
`GPU[0].Core[1]`, `Mesh[0][1]`.

## Validation

`MustBeValid` panics if a name does not follow the convention. The rules:

1. Names are hierarchical and dot-separated (`"A.B.C"` is valid, `"A.B.C."` is
   not).
2. Individual element names must not be empty (`"A..B"` is invalid).
3. Element names must be capitalized CamelCase — they must start with a capital
   letter (`"A.b"` is invalid, and `"gpu0"` fails because it is lowercase).
4. Element names must not contain `_`, `"`, `'`, or `-`.
5. Series elements use square-bracket notation, which must be balanced.

```go
naming.MustBeValid("GPU[0].Core[1]") // ok
naming.MustBeValid("GPU-0")          // panics: '-' is not allowed
naming.MustBeValid("gpu0")           // panics: must start with a capital letter
```

## Building Names

Helpers assemble child names from a parent name:

```go
naming.BuildName("", "GPU")                                  // "GPU"
naming.BuildName("GPU", "Core")                              // "GPU.Core"
naming.BuildNameWithIndex("GPU", "Core", 0)                  // "GPU.Core[0]"
naming.BuildNameWithMultiDimensionalIndex("GPU", "Core", []int{0, 1}) // "GPU.Core[0][1]"
```
