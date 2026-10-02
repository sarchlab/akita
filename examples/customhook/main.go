// Command customhook shows how to define your own hook point.
//
// The built-in hook points (engine events, port messages) only expose
// framework-level activity. To let an observer watch a component's own
// internal behavior, the component defines its own HookPos and calls
// InvokeHook at the right moment. Here a random walker fires a "step" hook on
// every step; an external hook logs the steps without the walker printing
// anything itself.
package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// HookPosStep is our own hook position. It fires once per walk step.
var HookPosStep = &hooking.HookPos{Name: "WalkStep"}

// walkStep is the payload carried in HookCtx.Item when HookPosStep fires.
type walkStep struct {
	Position int
	Steps    int
}

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

// Resources holds the random source the walker draws its steps from.
type Resources struct {
	RNG *rand.Rand
}

// Ports is empty: the walker talks to no one.
type Ports struct{}

// Middlewares holds the walker's behavior.
type Middlewares struct {
	// Walk takes one step per tick and reports it on HookPosStep.
	Walk *walkMW
}

// Comp is the walker, a ticking component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]

// Definition declares the walker.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz, WallDistance: 3},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Walk: &walkMW{comp: c}}
}

type walkMW struct {
	comp *Comp
}

func (m *walkMW) Handle(_ timing.Event) bool {
	s := &m.comp.State
	wall := m.comp.Spec.WallDistance

	if s.Position >= wall || s.Position <= -wall {
		return false
	}

	if m.comp.Resources.RNG.Intn(2) == 0 {
		s.Position--
	} else {
		s.Position++
	}
	s.Steps++

	// Fire our own hook point. Anything that has accepted a hook on this
	// component now sees the step.
	m.comp.InvokeHook(hooking.HookCtx{
		Domain: m.comp,
		Pos:    HookPosStep,
		Item:   walkStep{Position: s.Position, Steps: s.Steps},
	})

	return true
}

// stepLogger is an external hook that prints every step it is told about.
type stepLogger struct{}

func (h *stepLogger) Func(ctx hooking.HookCtx) {
	if ctx.Pos != HookPosStep {
		return
	}
	step := ctx.Item.(walkStep)
	fmt.Printf("[step %d] position %+d\n", step.Steps, step.Position)
}

func main() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	walker := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{RNG: rand.New(rand.NewSource(1))}).
		Build("Walker")

	// Observe the walker's own steps.
	walker.AcceptHook(&stepLogger{})

	walker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}
}
