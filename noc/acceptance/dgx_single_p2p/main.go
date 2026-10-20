package main

import (
	"flag"
	"fmt"
	"math/rand"

	acceptance "github.com/sarchlab/akita/v5/internal/nocacceptance"
	"github.com/sarchlab/akita/v5/noc/nvlink"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

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
	t.GenerateMsgs(1000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	if err := s.Terminate(); err != nil {
		panic(err)
	}
}

func createNetwork(s *sim.Simulation, test *acceptance.Test) {
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 9; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = twowaybuffered.MakeBuilder().WithSimulation(s).Build()
		}
		agent := acceptance.NewAgent(s, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	connector := nvlink.MakeBuilder().
		WithSimulation(s).Build("Network")

	rootComplexID := connector.AddRootComplex(agents[8].AgentPorts)
	switch1ID := connector.AddSwitch()
	switch2ID := connector.AddSwitch()

	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch1ID)
	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch2ID)

	for i := 0; i < 4; i++ {
		connector.ConnectDevice(switch1ID, agents[i].AgentPorts)
	}

	for i := 4; i < 8; i++ {
		connector.ConnectDevice(switch2ID, agents[i].AgentPorts)
	}

	connector.ConnectDevicesWithNVLink(0, 1)
	connector.ConnectDevicesWithNVLink(0, 2)
	connector.ConnectDevicesWithNVLink(1, 3)
	connector.ConnectDevicesWithNVLink(2, 3)
	connector.ConnectDevicesWithNVLink(0, 3)
	connector.ConnectDevicesWithNVLink(1, 2)
	connector.ConnectDevicesWithNVLink(5, 4)
	connector.ConnectDevicesWithNVLink(5, 7)
	connector.ConnectDevicesWithNVLink(7, 6)
	connector.ConnectDevicesWithNVLink(4, 6)
	connector.ConnectDevicesWithNVLink(4, 7)
	connector.ConnectDevicesWithNVLink(5, 6)
	connector.ConnectDevicesWithNVLink(0, 4)
	connector.ConnectDevicesWithNVLink(1, 5)
	connector.ConnectDevicesWithNVLink(2, 6)
	connector.ConnectDevicesWithNVLink(3, 7)

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[7])
}
