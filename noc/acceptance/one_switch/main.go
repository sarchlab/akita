package main

import (
	"flag"
	"fmt"
	"math/rand"

	acceptance "github.com/sarchlab/akita/v5/internal/nocacceptance"
	nc "github.com/sarchlab/akita/v5/noc/networkconnector"
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
	t.GenerateMsgs(20000)

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

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = twowaybuffered.MakeBuilder().WithSimulation(s).Build()
		}
		agent := acceptance.NewAgent(s, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	connector := nc.MakeBuilder().
		WithSimulation(s).
		WithFreq(freq).
		WithFlitSize(16).Build("Network")

	switchID := connector.AddSwitch(nc.SwitchConfig{})

	for _, agent := range agents {
		connector.ConnectDevice(
			switchID,
			agent.AgentPorts,
			nc.DeviceConfig{
				DeviceEndParam: nc.LinkEndDeviceParameter{
					NumInputChannel:  1,
					NumOutputChannel: 1,
				},
				SwitchEndParam: nc.LinkEndSwitchParameter{NumInputChannel: 1,
					NumOutputChannel: 1,
				},
			})
	}
}
