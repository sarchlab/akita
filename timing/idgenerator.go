package timing

import "sync/atomic"

// IDGenerator is an atomic counter owned by one simulation. Its zero value is
// ready to use; the first ID is 1. IDs are unique
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

// Name identifies the generator in its simulation's checkpoint inventory.
func (g *IDGenerator) Name() string { return "IDGenerator" }
