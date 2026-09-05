package timing

import (
	"sync"
	"sync/atomic"
)

// IDGenerator is owned by an engine. Share it with that engine's components and
// message producers; never share it between independently restorable simulations.
type IDGenerator interface {
	Generate() uint64
	// Track, Lookup, and Forget maintain owned ID associations (for example,
	// tracing tasks). They must be safe for concurrent use with Generate.
	Track(namespace string, key uint64) uint64
	Lookup(namespace string, key uint64) (uint64, bool)
	Forget(namespace string, key uint64)
}

// NewIDGenerator constructs an independent, checkpointable sequence.
func NewIDGenerator() *sequentialIDGenerator { return &sequentialIDGenerator{} }

type sequentialIDGenerator struct {
	nextID     uint64
	trackingMu sync.Mutex
	tracking   map[string]map[uint64]uint64
}

func (g *sequentialIDGenerator) Generate() uint64 { return atomic.AddUint64(&g.nextID, 1) }
func (g *sequentialIDGenerator) Name() string     { return "IDGenerator" }
func (g *sequentialIDGenerator) NextID() uint64   { return atomic.LoadUint64(&g.nextID) }

// Track associates a message with an owned tracing task, creating it if absent.
func (g *sequentialIDGenerator) Track(domain string, msg uint64) uint64 {
	g.trackingMu.Lock()
	defer g.trackingMu.Unlock()
	if g.tracking == nil {
		g.tracking = make(map[string]map[uint64]uint64)
	}
	if g.tracking[domain] == nil {
		g.tracking[domain] = make(map[uint64]uint64)
	}
	if id, ok := g.tracking[domain][msg]; ok {
		return id
	}
	id := g.Generate()
	g.tracking[domain][msg] = id
	return id
}
func (g *sequentialIDGenerator) Lookup(domain string, msg uint64) (uint64, bool) {
	g.trackingMu.Lock()
	defer g.trackingMu.Unlock()
	id, ok := g.tracking[domain][msg]
	return id, ok
}
func (g *sequentialIDGenerator) Forget(domain string, msg uint64) {
	g.trackingMu.Lock()
	defer g.trackingMu.Unlock()
	delete(g.tracking[domain], msg)
	if len(g.tracking[domain]) == 0 {
		delete(g.tracking, domain)
	}
}

// IDSource exposes the sequence used for event, message, and tracing IDs.
type IDSource interface{ IDGenerator() IDGenerator }

// IDsFor obtains an explicitly supplied owner's sequence. Custom event
// schedulers must implement IDSource when used to construct Akita components.
func IDsFor(owner any) IDGenerator { return owner.(IDSource).IDGenerator() }
