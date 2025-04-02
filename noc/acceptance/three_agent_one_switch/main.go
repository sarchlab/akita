package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/noc/acceptance"
	nc "github.com/sarchlab/akita/v4/noc/networking/networkconnector"
	"github.com/sarchlab/akita/v4/sim"
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

func createNetwork(engine sim.Engine, test *acceptance.Test) {
	freq := 1.0 * sim.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 3; i++ {
		agent := acceptance.NewAgent(
			engine, freq, fmt.Sprintf("Agent%d", i), 5, test)
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
					IncomingBufSize:  2,
					OutgoingBufSize:  2,
					NumInputChannel:  4,
					NumOutputChannel: 4,
				},
				SwitchEndParam: nc.LinkEndSwitchParameter{
					IncomingBufSize:  2,
					OutgoingBufSize:  2,
					NumInputChannel:  4,
					NumOutputChannel: 4,
				},
				LinkParam: nc.LinkParameter{
					IsIdeal:   true,
					Frequency: freq,
				},
			})
	}

	connector.EstablishRoute()
}
