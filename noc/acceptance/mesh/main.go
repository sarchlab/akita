package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/noc/acceptance"
	"github.com/sarchlab/akita/v4/noc/networking/mesh"
	"github.com/sarchlab/akita/v4/sim"
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
	engine := sim.NewSerialEngine()

	monitor := monitoring.NewMonitor()
	monitor.RegisterEngine(engine)

	freq := 1 * sim.GHz
	connector := mesh.NewConnector().
		WithMonitor(monitor).
		WithEngine(engine).
		WithFreq(freq)

	connector.CreateNetwork("Mesh")

	for x := 0; x < meshWidth; x++ {
		for y := 0; y < meshHeight; y++ {
			name := fmt.Sprintf("Agent[%d][%d]", x, y)
			agent := acceptance.NewAgent(engine, freq, name, 1, test)
			agent.TickLater()

			monitor.RegisterComponent(agent)

			connector.AddTile([3]int{x, y, 0}, agent.AgentPorts)
			test.RegisterAgent(agent)
		}
	}

	connector.EstablishNetwork()

	test.GenerateMsgs(uint64(numMessages))

	monitor.StartServer()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	test.MustHaveReceivedAllMsgs()
	test.ReportBandwidthAchieved(engine.CurrentTime())
	fmt.Println("passed!")
}
