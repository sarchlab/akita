package main

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/v5/sim"
	"github.com/sarchlab/akita/v4/v5/timing"
)

var (
	endCycle = timing.VTimeInCycle(10)
	randGen  *rand.Rand
)

// stateHandle reads the committed snapshot for a specific state slot.
type stateHandle[StateType any] interface {
	Load() StateType
	MarkStateUpdate(StateType)
}

type splitEvent struct {
	cycle   timing.VTimeInCycle
	id      int
	handler *splitHandler
}

func (e splitEvent) Time() timing.VTimeInCycle { return e.cycle }

func (e splitEvent) Handler() timing.Handler { return e.handler }

// splitHandler stages population updates using a single state representation.
type splitHandler struct {
	rng        *rand.Rand
	engine     timing.Engine
	population stateHandle[CellSplitState]
}

func (h *splitHandler) Handle(evt timing.Event) {
	split := evt.(splitEvent)

	state := h.population.Load()
	state.Cells += 1
	h.population.MarkStateUpdate(state)

	jitter := func() timing.VTimeInCycle {
		return timing.VTimeInCycle(h.rng.Intn(3) + 1)
	}

	h.engine.Schedule(splitEvent{
		cycle:   evt.Time() + jitter(),
		id:      split.id,
		handler: h,
	})

	h.engine.Schedule(splitEvent{
		cycle:   evt.Time() + jitter(),
		id:      state.Cells,
		handler: h,
	})
}

// CellSplitState represents the mutable population description for the sample.
type CellSplitState struct {
	Cells int
}

func main() {
	randGen = rand.New(rand.NewSource(0))

	simulation := sim.MakeBuilder().
		Build()

	engine := simulation.GetEngine()

	populationHandle := simulation.MustRegisterStateHandle("cells/population", CellSplitState{
		Cells: 1,
	}).(stateHandle)

	handler := &splitHandler{
		rng:        randGen,
		population: populationHandle,
	}

	firstCycle := timing.VTimeInCycle(randGen.Intn(3) + 1)
	engine.Schedule(splitEvent{
		cycle:   firstCycle,
		id:      0,
		handler: handler,
	})

	simulation.Run()

	snapshot := populationHandle.Load()
	fmt.Printf("Cell count at cycle %d: %d\n", endCycle, snapshot.Cells)
}
