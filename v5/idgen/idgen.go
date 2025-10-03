// Package idgen provides deterministic-friendly ID generators for Akita V5.
package idgen

import "sync/atomic"

// ID is a unique identifier represented as a uint64.
type ID uint64

// Generator produces unique identifiers.
type Generator interface {
	Generate() ID
}

// New returns a sequential generator whose first emitted ID is "1".
func New() Generator {
	return &sequentialGenerator{}
}

type sequentialGenerator struct {
	next uint64
}

func (g *sequentialGenerator) Generate() ID {
	return ID(atomic.AddUint64(&g.next, 1))
}
