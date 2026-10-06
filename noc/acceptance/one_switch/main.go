package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	nc "github.com/sarchlab/akita/v5/noc/networking/networkconnector"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	s := acceptance.NewSimulation()
	engine := s.Engine()
	t := acceptance.NewTest()

	createNetwork(s, t)
	t.GenerateMsgs(20000)

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

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = messaging.NewPort(fmt.Sprintf("%s.Port%d", name, j), 1, 1)
		}
		agent := acceptance.NewAgent(s, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	connector := nc.MakeConnector().
		WithSimulation(s).
		WithDefaultFreq(freq).
		WithFlitSize(16)

	connector.NewNetwork("Network")

	switchID := connector.AddSwitch()

	for _, agent := range agents {
		connector.ConnectDevice(
			switchID,
			agent.AgentPorts,
			nc.DeviceToSwitchLinkParameter{
				DeviceEndParam: nc.LinkEndDeviceParameter{
					IncomingBufSize:  1,
					OutgoingBufSize:  1,
					NumInputChannel:  1,
					NumOutputChannel: 1,
				},
				SwitchEndParam: nc.LinkEndSwitchParameter{
					IncomingBufSize:  1,
					OutgoingBufSize:  1,
					NumInputChannel:  1,
					NumOutputChannel: 1,
				},
				LinkParam: nc.LinkParameter{
					IsIdeal:   true,
					Frequency: freq,
				},
			})
	}

	connector.EstablishRoute()
}
