package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type helloSpec struct {
	NumTicks int `json:"num_ticks"`
}

type helloState struct {
	Cycle int `json:"cycle"`
}

type helloMW struct {
	comp *modeling.Component[helloSpec, helloState, modeling.None]
}

func (m *helloMW) Tick() bool {
	state := &m.comp.State
	if state.Cycle >= m.comp.Spec().NumTicks {
		return false
	}

	fmt.Printf("tick %d at %d ps\n", state.Cycle, m.comp.CurrentTime())
	state.Cycle++

	return true
}

func main() {
	s := simulation.MakeBuilder().Build()

	comp := modeling.NewBuilder[helloSpec, helloState, modeling.None]().
		WithEngine(s.GetEngine()).
		WithFreq(1 * timing.GHz).
		WithSpec(helloSpec{NumTicks: 3}).
		Build("Hello")
	comp.AddMiddleware(&helloMW{comp: comp})

	comp.TickLater()

	if err := s.GetEngine().Run(); err != nil {
		panic(err)
	}

	s.Terminate()
}
