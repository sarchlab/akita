package mem

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// StorageBuilder builds Storage resources. WithSimulation is required; Build registers the resource.
type StorageBuilder struct {
	capacity   uint64
	unitSize   uint64
	simulation modeling.Simulation
}

// MakeStorageBuilder returns a StorageBuilder with a default 4 KB unit size.
func MakeStorageBuilder() StorageBuilder {
	return StorageBuilder{
		unitSize: 4 * KB,
	}
}

// WithCapacity sets the capacity of the storage in bytes.
func (b StorageBuilder) WithCapacity(capacity uint64) StorageBuilder {
	b.capacity = capacity
	return b
}

// WithUnitSize sets the allocation unit size in bytes. A smaller unit reduces
// memory consumption for sparsely accessed storage.
func (b StorageBuilder) WithUnitSize(unitSize uint64) StorageBuilder {
	b.unitSize = unitSize
	return b
}

// WithSimulation wires the builder to a simulation so the built storage
// registers itself as a resource.
func (b StorageBuilder) WithSimulation(sim modeling.Simulation) StorageBuilder {
	b.simulation = sim
	return b
}

// Build creates and registers the resource. WithSimulation is required.
func (b StorageBuilder) Build(name string) *Storage {
	if b.simulation == nil {
		panic("Storage: WithSimulation is required")
	}
	storage := NewStorageWithUnitSize(b.capacity, b.unitSize)
	storage.name = name

	b.simulation.RegisterResource(storage)

	return storage
}
