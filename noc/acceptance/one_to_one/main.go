package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/switching/endpoint"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	s := simulation.MakeBuilder().WithoutMonitoring().Build()
	engine := s.GetEngine()
	t := acceptance.NewTest()

	createNetwork(s, t)
	t.GenerateMsgs(20000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	atexit.Exit(0)
}

func createNetwork(s *simulation.Simulation, test *acceptance.Test) {
	engine := s.GetEngine()
	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("Agent%d", i)
		ports := make([]messaging.Port, 5)
		for j := 0; j < 5; j++ {
			ports[j] = messaging.NewPort(nil, 1, 1, fmt.Sprintf("%s.Port%d", name, j))
		}
		agent := acceptance.NewAgent(engine, freq, name, ports, test)
		agent.TickLater()
		agents = append(agents, agent)
	}

	epSpec := endpoint.DefaultSpec()
	epSpec.Freq = freq
	epSpec.FlitByteSize = 8

	ep1 := endpoint.MakeBuilder().
		WithRegistrar(s).
		WithSpec(epSpec).
		WithResources(endpoint.Resources{DevicePorts: agents[0].AgentPorts}).
		Build("EP1")

	ep2 := endpoint.MakeBuilder().
		WithRegistrar(s).
		WithSpec(epSpec).
		WithResources(endpoint.Resources{DevicePorts: agents[1].AgentPorts}).
		Build("EP2")

	ep1.SetDefaultSwitchDst(ep2.NetworkPort().AsRemote())
	ep2.SetDefaultSwitchDst(ep1.NetworkPort().AsRemote())

	conn := directconnection.MakeBuilder().
		WithRegistrar(s).
		Build("Conn")

	conn.PlugIn(ep1.NetworkPort())
	conn.PlugIn(ep2.NetworkPort())

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[1])
}
