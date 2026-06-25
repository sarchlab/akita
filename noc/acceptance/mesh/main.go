package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/mesh"
	"github.com/sarchlab/akita/v5/timing"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	// In case of a panic, do not exit the program, but sleep.
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		fmt.Println("Recovered from panic:", r)
	// 		select {}
	// 	}
	// }()

	meshWidth := 2
	meshHeight := 2
	numMessages := 100

	test := acceptance.NewTest()
	sim := acceptance.NewSimulation()
	engine := sim.GetEngine()

	freq := 1 * timing.GHz
	connector := mesh.NewConnector().
		WithRegistrar(sim).
		WithFreq(freq)

	connector.CreateNetwork("Mesh")

	for x := 0; x < meshWidth; x++ {
		for y := 0; y < meshHeight; y++ {
			name := fmt.Sprintf("Agent[%d][%d]", x, y)
			ports := []messaging.Port{
				messaging.NewPort(nil, 1, 1, name+".Port0"),
			}
			agent := acceptance.NewAgent(sim, freq, name, ports, test)
			agent.TickLater()

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
	test.ReportBandwidthAchieved(engine.CurrentTime())
	sim.Terminate()
	fmt.Println("passed!")
}
