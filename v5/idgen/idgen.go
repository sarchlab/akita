// Package idgen provides deterministic-friendly ID generators for Akita V5.
//
// The V4 simulator exposed a global ID generator whose configuration was shared
// across the entire process. In V5 we prefer explicit ownership: each
// simulation (or other subsystem) should create its own generator so concurrent
// simulations do not interfere with each other's sequencing or determinism.
// Generators are intentionally tiny and safe for concurrent use.
package idgen

import "sync/atomic"

// ID is a unique identifier represented as a uint64.
type ID uint64

// Generator is a sequential, concurrency-safe numeric ID generator.
type Generator struct {
	next uint64
}

// New returns a sequential generator whose first emitted ID is `1`.
//
// The returned generator is safe for concurrent use. Convert IDs to strings
// with `strconv.FormatUint(uint64(id), 10)` when needed for logging or tracing.
func New() *Generator {
	return &Generator{}
}

// Generate returns the next ID in the sequence.
func (g *Generator) Generate() ID {
	return ID(atomic.AddUint64(&g.next, 1))
}
