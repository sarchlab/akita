package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type walkSpec struct {
	Freq         timing.Freq `json:"freq"`
	WallDistance int         `json:"wall_distance"`
}

type walkState struct {
	Position int `json:"position"`
	Steps    int `json:"steps"`
}

// Comp is the random-walk component. The alias keeps the long generic
// modeling.Component[Spec, State, Resources] type out of the rest of the code.
type Comp = modeling.Component[walkSpec, walkState, modeling.None]

type walkMW struct {
	comp *Comp
	rng  *rand.Rand
}

func (m *walkMW) Tick() bool {
	state := &m.comp.State
	wall := m.comp.Spec().WallDistance

	if state.Position >= wall || state.Position <= -wall {
		fmt.Printf("hit wall at %+d after %d steps (%d ps)\n",
			state.Position, state.Steps, m.comp.CurrentTime())
		return false
	}

	if m.rng.Intn(2) == 0 {
		state.Position--
	} else {
		state.Position++
	}
	state.Steps++

	return true
}

func main() {
	s := simulation.MakeBuilder().Build()

	spec := walkSpec{Freq: 1 * timing.GHz, WallDistance: 10}
	comp := modeling.NewBuilder[walkSpec, walkState, modeling.None]().
		WithEngine(s.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build("Walker")
	comp.AddMiddleware(&walkMW{
		comp: comp,
		rng:  rand.New(rand.NewSource(1)),
	})

	comp.TickLater()

	if err := s.GetEngine().Run(); err != nil {
		panic(err)
	}

	s.Terminate()
}
