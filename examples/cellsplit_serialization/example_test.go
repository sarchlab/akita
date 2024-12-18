package cellsplit

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func config(sim *simulation.Simulation) {
	engine := timing.NewSerialEngine()
	sim.RegisterEngine(engine)

	handler := &SplitHandler{
		name:    "handler",
		rand:    rand.New(rand.NewSource(1)),
		engine:  sim.GetEngine(),
		endTime: timing.VTimeInSec(100),
	}
	sim.RegisterLocation(handler)
}

func firstStage() {
	sim := simulation.NewSimulation()
	config(sim)

	sim.GetEngine().Schedule(splitEvent{
		id:      id.Generate(),
		time:    0,
		handler: sim.GetLocation("handler").(timing.Handler),
	})

	sim.GetEngine().Run()

	sim.Save("sim.json")
}

func secondStage() {
	sim := simulation.NewSimulation()
	config(sim)

	sim.Load("sim.json")

	handler := sim.GetLocation("handler").(*SplitHandler)
	handler.endTime = timing.VTimeInSec(200)

	engine := sim.GetEngine()
	engine.Schedule(splitEvent{
		id:      id.Generate(),
		time:    engine.Now(),
		handler: sim.GetLocation("handler").(timing.Handler),
	})

	engine.Run()

	fmt.Printf("Total number at time 10: %d\n", handler.total)
}

func Example() {
	firstStage()
	secondStage()

	// Output: Total number at time 10: 69
}
