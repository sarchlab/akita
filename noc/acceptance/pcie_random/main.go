package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/pcie"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/tebeka/atexit"
)

var numDevicePerSwitch = 8
var numPortPerDevice = 9

func main() {
	flag.Parse()
	rand.Seed(1)

	s := acceptance.NewSimulation()
	engine := s.Engine()

	t := acceptance.NewTest()

	createNetwork(s, t)
	if err := s.Initialize(); err != nil {
		panic(err)
	}
	t.GenerateMsgs(10000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	s.Terminate()
	atexit.Exit(0)
}

func createNetwork(s *sim.Simulation, test *acceptance.Test) {
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < numDevicePerSwitch*2+1; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, numPortPerDevice)
		for j := 0; j < numPortPerDevice; j++ {
			ports[j] = twowaybuffered.NewPort(1, 1)
		}
		agent := acceptance.NewAgent(s, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithSimulation(s).
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
