package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/switching/endpoint"
	"github.com/sarchlab/akita/v5/sim"
	simengine "github.com/sarchlab/akita/v5/sim/engine"
	"github.com/sarchlab/akita/v5/sim/directconnection"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := simengine.NewSerialEngine()
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

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent%d", i)
		ports := make([]sim.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = sim.NewPort(nil, 1, 1, fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(engine, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	ep1 := endpoint.MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		WithFlitByteSize(8).
		WithDevicePorts(agents[0].AgentPorts).
		WithNetworkPort(sim.NewPort(nil, 4, 4, "EP1.NetworkPort")).
		Build("EP1")

	ep2 := endpoint.MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		WithFlitByteSize(8).
		WithDevicePorts(agents[1].AgentPorts).
		WithNetworkPort(sim.NewPort(nil, 4, 4, "EP2.NetworkPort")).
		Build("EP2")

	ep1.SetDefaultSwitchDst(ep2.NetworkPort().AsRemote())
	ep2.SetDefaultSwitchDst(ep1.NetworkPort().AsRemote())

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		Build("Conn")

	conn.PlugIn(ep1.NetworkPort())
	conn.PlugIn(ep2.NetworkPort())

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[1])
}
