// Command 03_random_walk is the smallest useful Akita simulation: one ticking
// component, no ports. The walker takes one ±1 step per cycle until it drifts
// WallDistance away from the origin, then reports where and when it stopped.
package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec is the walker's configuration.
type Spec struct {
	Freq         timing.Freq `json:"freq"`
	WallDistance int         `json:"wall_distance"`
}

// State is the walker's runtime data.
type State struct {
	Position int `json:"position"`
	Steps    int `json:"steps"`
}

// Resources holds the random source the walker draws its steps from. The
// system builder supplies it, so it also chooses the seed.
type Resources struct {
	RNG *rand.Rand
}

// Ports is empty: the walker talks to no one.
type Ports struct{}

// Middlewares holds the walker's behavior.
type Middlewares struct {
	// Walk takes one step per tick until the walker hits a wall.
	Walk *walkMW
}

// Comp is the walker, a ticking component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]

// Definition declares the walker.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz, WallDistance: 10},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Walk: &walkMW{comp: c}}
}

type walkMW struct {
	comp *Comp
}

// Handle takes one step. It makes no progress once the walker is at a wall,
// so the component stops ticking and the simulation ends.
func (m *walkMW) Handle(_ timing.Event) bool {
	state := &m.comp.State
	wall := m.comp.Spec.WallDistance

	if state.Position >= wall || state.Position <= -wall {
		fmt.Printf("hit wall at %+d after %d steps (%d ps)\n",
			state.Position, state.Steps, m.comp.CurrentTime())
		return false
	}

	if m.comp.Resources.RNG.Intn(2) == 0 {
		state.Position--
	} else {
		state.Position++
	}
	state.Steps++

	return true
}

func main() {
	s := sim.MakeBuilder().Build()

	walker := Definition.Builder().
		WithSimulation(s).
		WithResources(Resources{RNG: rand.New(rand.NewSource(1))}).
		Build("Walker")

	// Nothing wakes a component with no ports, so start it explicitly.
	walker.TickLater()

	if err := s.Engine().Run(); err != nil {
		panic(err)
	}

	s.Terminate()
}
