package timing

import "sync/atomic"

// IDSource allocates IDs in one simulation's namespace. The generator identity
// also distinguishes runtime namespaces when different simulations reuse IDs.
type IDSource interface {
	NewID() uint64
	GetIDGenerator() *IDGenerator
}

// IDGenerator is an atomic counter owned by one simulation (or standalone
// engine). Its zero value is ready to use; the first ID is 1. IDs are unique
// within this generator, not across simulations. Do not copy a generator after
// use. Parallel callers receive unique IDs, but their allocation order depends
// on execution order.
type IDGenerator struct {
	nextID uint64
}

// NewID returns the next ID in this generator's namespace.
func (g *IDGenerator) NewID() uint64 {
	return atomic.AddUint64(&g.nextID, 1)
}

// GetIDGenerator returns this generator. It lets an explicitly supplied
// generator serve as an IDSource in helpers and isolated tests.
func (g *IDGenerator) GetIDGenerator() *IDGenerator { return g }

// Name identifies the generator in its simulation's checkpoint inventory.
func (g *IDGenerator) Name() string { return "IDGenerator" }
