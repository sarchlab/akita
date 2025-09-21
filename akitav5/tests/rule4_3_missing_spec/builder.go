//go:build ignore
// +build ignore

package rule4_3_missing_spec

import "github.com/sarchlab/akita/v4/simv5"

type Builder struct {
	simulation *simv5.Simulation
}

func MakeBuilder() Builder { return Builder{} }

func (b Builder) WithSimulation(sim *simv5.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec is intentionally missing to trigger Rule 4.3.

func (b Builder) Build(name string) *Comp {
	if err := b.spec.validate(); err != nil {
		panic(err)
	}
	return &Comp{}
}
