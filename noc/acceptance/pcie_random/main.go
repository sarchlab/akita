package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/pcie"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/tebeka/atexit"
)

var numDevicePerSwitch = 8
var numPortPerDevice = 9

func main() {
	flag.Parse()
	rand.Seed(1)

	sim := acceptance.NewSimulation()
	engine := sim.GetEngine()

	t := acceptance.NewTest()

	createNetwork(sim, t)
	t.GenerateMsgs(10000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	sim.Terminate()
	atexit.Exit(0)
}

func createNetwork(sim *simulation.Simulation, test *acceptance.Test) {
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < numDevicePerSwitch*2+1; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, numPortPerDevice)
		for j := 0; j < numPortPerDevice; j++ {
			ports[j] = messaging.NewPort(nil, 1, 1,
				fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(sim, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithRegistrar(sim).
		WithFrequency(freq).
		WithVersion(4, 16)

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(agents[0].AgentPorts)
	switch1ID := pcieConnector.AddSwitch(rootComplexID)

	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch1ID, agents[i+1].AgentPorts)
	}

	switch2ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch2ID,
			agents[i+1+numDevicePerSwitch].AgentPorts)
	}

	pcieConnector.EstablishRoute()
}
