package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	nc "github.com/sarchlab/akita/v5/noc/networking/networkconnector"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	sim := acceptance.NewSimulation()
	engine := sim.GetEngine()
	t := acceptance.NewTest()

	createNetwork(sim, t)
	t.GenerateMsgs(20000)

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

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent[%d]", i)
		ports := make([]messaging.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = messaging.NewPort(nil, 1, 1, fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(sim, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	connector := nc.MakeConnector().
		WithRegistrar(sim).
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
