package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
)

var (
	endTime = sim.VTimeInSec(10)
	engine  twoPhaseEngine
	randGen *rand.Rand
)

// twoPhaseEngine sketches the engine contract that defers visible state changes
// until a commit phase.
type twoPhaseEngine interface {
	sim.Engine

	StageEvent(sim.Event)
	RunUntil(sim.VTimeInSec) error
}

// stateHandle gives access to the committed snapshot. Handlers return the
// mutated value, and the controller commits it after all stage work completes.
type stateHandle interface {
	Load() StateType
	Slot() string
}

type splitEvent struct {
	tick    sim.VTimeInSec
	id      int
	handler *splitHandler
}

func (e splitEvent) Time() sim.VTimeInSec { return e.tick }

func (e splitEvent) Handler() sim.Handler { return e.handler }

// splitHandler stages population updates using a single state representation.
type splitHandler struct {
	rng        *rand.Rand
	population stateHandle
}

func (h *splitHandler) Handle(ctx cycleCtx, event sim.Event) (any, error) {
	split := event.(splitEvent)

	snapshot := h.population.Load()
	snapshot.Log(ctx.Now(), split.id)

	nextState, newID := snapshot.NextState(split.id)

	schedule := func(id int) {
		jitter := sim.VTimeInSec(h.rng.Float64() + 1)
		ctx.StageEvent(splitEvent{
			tick:    ctx.Now() + jitter,
			id:      id,
			handler: h,
		})
	}

	schedule(split.id)
	schedule(newID)

	return nextState, nil
}

// StateType represents the mutable population description for the sample.
type StateType struct {
	Cells int
}

func (s StateType) Log(now sim.VTimeInSec, eventID int) {
	fmt.Printf("Cell %d split at %.10f, observed count: %d\n", eventID, now, s.Cells)
}

func (s StateType) NextState(_ int) (StateType, int) {
	next := s
	newID := s.Cells
	next.Cells++
	return next, newID
}

func main() {
	randGen = rand.New(rand.NewSource(0))

	sim := simulation.MakeBuilder().
		WithTwoPhaseState().
		Build()

	engine = sim.GetEngine().(twoPhaseEngine)

	populationHandle := sim.MustRegisterStateHandle("cells/population", StateType{
		Cells: 1,
	}).(stateHandle)

	handler := &splitHandler{
		rng:        randGen,
		population: populationHandle,
	}

	firstTick := sim.VTimeInSec(randGen.Float64() + 1)
	engine.StageEvent(splitEvent{
		tick:    firstTick,
		id:      0,
		handler: handler,
	})

	_ = engine.RunUntil(endTime)

	snapshot := populationHandle.Load()
	fmt.Printf("Cell count at time %.0f: %d\n", endTime, snapshot.Cells)
}
