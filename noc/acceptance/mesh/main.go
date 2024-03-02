package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/noc/acceptance"
	"github.com/sarchlab/akita/v4/noc/networking/mesh"
	"github.com/sarchlab/akita/v4/sim"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	meshWidth := 5
	meshHeight := 5
	numMessages := 2000

	test := acceptance.NewTest()
	engine := sim.NewSerialEngine()

	freq := 1 * sim.GHz
	connector := mesh.NewConnector().
		// WithMonitor(monitor).
		WithEngine(engine).
		WithFreq(freq)

	connector.CreateNetwork("Mesh")

	for x := 0; x < meshWidth; x++ {
		for y := 0; y < meshHeight; y++ {
			name := fmt.Sprintf("Agent[%d][%d]", x, y)
			agent := acceptance.NewAgent(engine, freq, name, 4, test)
			agent.TickLater(0)

			// monitor.RegisterComponent(agent)

			connector.AddTile([3]int{x, y, 0}, agent.AgentPorts)
			test.RegisterAgent(agent)
		}
	}

	connector.EstablishNetwork()

	test.GenerateMsgs(uint64(numMessages))

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	test.MustHaveReceivedAllMsgs()
	fmt.Println("passed!")
}
