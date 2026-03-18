package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/pcie"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := sim.NewSerialEngine()
	t := acceptance.NewTest()

	createNetwork(engine, t)
	t.GenerateMsgs(1000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	atexit.Exit(0)
}

func createNetwork(engine sim.EventScheduler, test *acceptance.Test) {
	freq := 1.0 * sim.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 9; i++ {
		name := fmt.Sprintf("Agent%d", i)
		ports := make([]sim.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = sim.NewPort(nil, 1, 1, fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(engine, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithEngine(engine).
		WithFrequency(1*sim.GHz).
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
