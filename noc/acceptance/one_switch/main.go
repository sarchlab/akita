package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	nc "github.com/sarchlab/akita/v5/noc/networking/networkconnector"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := sim.NewSerialEngine()
	t := acceptance.NewTest()

	createNetwork(engine, t)
	t.GenerateMsgs(20000)

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

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent%d", i)
		ports := make([]sim.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = sim.NewPort(nil, 1, 1, fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(engine, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
	}

	connector := nc.MakeConnector().
		WithEngine(engine).
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
