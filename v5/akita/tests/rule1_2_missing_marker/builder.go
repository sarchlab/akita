//go:build ignore
// +build ignore

package rule1_2_missing_marker

import simv5 "github.com/sarchlab/akita/v4/v5/sim"

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
	if err := b.spec.validate(); err != nil {
		panic(err)
	}
	return &Comp{}
}
