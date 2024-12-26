package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/noc/acceptance"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/noc/networking/switching/endpoint"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := timing.NewSerialEngine()
	sim := simulation.NewSimulation()
	sim.RegisterEngine(engine)

	t := acceptance.NewTest()

	createNetwork(sim, t)
	t.GenerateMsgs(20000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.Now())
	atexit.Exit(0)
}

func createNetwork(sim simulation.Simulation, test *acceptance.Test) {
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 2; i++ {
		agent := acceptance.NewAgent(
			sim, freq, fmt.Sprintf("Agent%d", i), 5, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	ep1 := endpoint.MakeBuilder().
		WithSimulation(sim).
		WithFreq(freq).
		WithFlitByteSize(8).
		WithDevicePorts(agents[0].AgentPorts).
		Build("EP1")

	ep2 := endpoint.MakeBuilder().
		WithSimulation(sim).
		WithFreq(freq).
		WithFlitByteSize(8).
		WithDevicePorts(agents[1].AgentPorts).
		Build("EP2")

	ep1.DefaultSwitchDst = ep2.NetworkPort.AsRemote()
	ep2.DefaultSwitchDst = ep1.NetworkPort.AsRemote()

	conn := directconnection.MakeBuilder().
		WithEngine(sim.GetEngine()).
		WithFreq(freq).
		Build("Conn")

	conn.PlugIn(ep1.NetworkPort)
	conn.PlugIn(ep2.NetworkPort)

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[1])
}
