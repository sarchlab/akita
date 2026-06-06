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
	"github.com/sarchlab/akita/v5/timing"
)

// HookPosStep is our own hook position. It fires once per walk step.
var HookPosStep = &hooking.HookPos{Name: "WalkStep"}

// walkStep is the payload carried in HookCtx.Item when HookPosStep fires.
type walkStep struct {
	Position int
	Steps    int
}

type walkSpec struct {
	WallDistance int `json:"wall_distance"`
}

type walkState struct {
	Position int `json:"position"`
	Steps    int `json:"steps"`
}

// Comp is the walker component.
type Comp = modeling.Component[walkSpec, walkState, modeling.None]

type walkMW struct {
	comp *Comp
	rng  *rand.Rand
}

func (m *walkMW) Tick() bool {
	s := &m.comp.State
	wall := m.comp.Spec().WallDistance

	if s.Position >= wall || s.Position <= -wall {
		return false
	}

	if m.rng.Intn(2) == 0 {
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
	registrar := modeling.NewStandaloneRegistrar(engine)

	walker := modeling.NewBuilder[walkSpec, walkState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(walkSpec{WallDistance: 3}).
		Build("Walker")
	walker.AddMiddleware(&walkMW{
		comp: walker,
		rng:  rand.New(rand.NewSource(1)),
	})
	registrar.RegisterComponent(walker)

	// Observe the walker's own steps.
	walker.AcceptHook(&stepLogger{})

	walker.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}
}
