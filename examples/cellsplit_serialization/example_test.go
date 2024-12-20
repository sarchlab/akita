package cellsplit

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func config(sim simulation.Simulation) {
	engine := timing.NewSerialEngine()
	sim.RegisterEngine(engine)

	var handler = &SplitHandler{
		state: &state{
			name:    "handler",
			endTime: timing.VTimeInSec(100),
		},
		rand:   rand.New(rand.NewSource(1)),
		engine: engine,
	}

	sim.RegisterStateHolder(handler)
}

func firstStage() {
	sim := simulation.NewSimulation()
	config(sim)

	handler := sim.GetStateHolder("handler").(*SplitHandler)
	sim.GetEngine().Schedule(&splitEvent{
		id:      id.Generate(),
		time:    0,
		handler: handler,
	})

	sim.GetEngine().Run()

	sim.Save("sim.json")
}

func secondStage() {
	sim := simulation.NewSimulation()
	config(sim)

	sim.Load("sim.json")

	handler := sim.GetStateHolder("handler").(*SplitHandler)
	handler.state.endTime = timing.VTimeInSec(200)

	engine := sim.GetEngine()
	engine.Schedule(&splitEvent{
		id:      id.Generate(),
		time:    100,
		handler: handler,
	})

	engine.Run()

	fmt.Printf("Total number at time %.1f: %d\n",
		engine.Now(),
		handler.total,
	)
}

func Example() {
	firstStage()
	secondStage()

	// Output: Total number at time 197.9: 138
}
