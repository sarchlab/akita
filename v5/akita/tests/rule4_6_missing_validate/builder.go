//go:build ignore
// +build ignore

package rule4_6_missing_validate

import "github.com/sarchlab/akita/v4/simv5"

type Builder struct {
	simulation *simv5.Simulation
	spec       Spec
}

func MakeBuilder() Builder { return Builder{spec: defaults()} }

func (b Builder) WithSimulation(sim *simv5.Simulation) Builder {
	b.simulation = sim
	return b
}

func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

func (b Builder) Build(name string) *Comp {
	// Missing validate() call.
	return &Comp{}
}
