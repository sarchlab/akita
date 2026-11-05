package mem

import "github.com/sarchlab/akita/v5/sim/modeling"

// StorageSpec declares a shared storage resource. Both sizes are explicit bytes.
type StorageSpec struct {
	Capacity uint64
	UnitSize uint64
}

// BuildResource constructs and registers storage during blueprint initialization.
func (s StorageSpec) BuildResource(sim modeling.Simulation, name string, _ modeling.None) *Storage {
	if s.Capacity == 0 || s.UnitSize == 0 {
		panic("StorageSpec: capacity and unit size must be positive")
	}
	return MakeStorageBuilder().WithSimulation(sim).WithCapacity(s.Capacity).WithUnitSize(s.UnitSize).Build(name)
}
