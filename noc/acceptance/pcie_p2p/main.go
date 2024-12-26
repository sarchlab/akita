package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/noc/acceptance"
	"github.com/sarchlab/akita/v4/noc/networking/pcie"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := timing.NewSerialEngine()
	sim := simulation.NewSimulation()
	sim.RegisterEngine(engine)

	t := acceptance.NewTest()

	createNetwork(sim, t)
	t.GenerateMsgs(10000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.Now())
	atexit.Exit(0)
}

func createNetwork(sim simulation.Simulation, test *acceptance.Test) {
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 9; i++ {
		agent := acceptance.NewAgent(
			sim, freq, fmt.Sprintf("Agent%d", i), 5, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithSimulation(sim).
		WithFrequency(1*timing.GHz).
		WithVersion(4, 16)

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(agents[0].AgentPorts)

	switch1ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 1; i < 5; i++ {
		pcieConnector.PlugInDevice(switch1ID, agents[i].AgentPorts)
	}

	switch2ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 5; i < 9; i++ {
		pcieConnector.PlugInDevice(switch2ID, agents[i].AgentPorts)
	}

	pcieConnector.EstablishRoute()

	test.RegisterAgent(agents[1])
	test.RegisterAgent(agents[8])
}
