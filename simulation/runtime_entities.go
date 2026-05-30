package simulation

import "github.com/sarchlab/akita/v5/timing"

// IDGeneratorHandle is the registered entity object for the global ID
// generator. It is a thin, stateless handle: it reads and writes the global
// sequential ID generator on demand and does not instantiate the generator
// merely by being registered. Phase B serialization uses it to snapshot and
// restore the generator's next-ID counter.
type IDGeneratorHandle struct{}

// NextID returns the next ID the global sequential ID generator will produce.
// It ensures the generator is instantiated first, so it is safe to call even
// before any ID has been generated.
func (IDGeneratorHandle) NextID() uint64 {
	timing.GetIDGenerator()
	return timing.GetIDGeneratorNextID()
}

// SetNextID sets the next ID the global sequential ID generator will produce.
// It ensures the generator is instantiated first.
func (IDGeneratorHandle) SetNextID(id uint64) {
	timing.GetIDGenerator()
	timing.SetIDGeneratorNextID(id)
}

// registerRuntimeSingletons registers the engine and the global ID generator
// as ordinary entities. They are singletons with reserved names, so the whole
// mutable runtime — including the engine and ID counter — is one addressable
// inventory rather than special-cased metadata.
func (b Builder) registerRuntimeSingletons(s *Simulation) {
	s.registerEntity(
		Entity{Kind: EntityKindEngine, Name: engineEntityName},
		s.engine,
	)

	s.registerEntity(
		Entity{Kind: EntityKindIDGenerator, Name: idGeneratorEntityName},
		IDGeneratorHandle{},
	)
}
