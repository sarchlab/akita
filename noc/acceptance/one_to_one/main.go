package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v5/noc/acceptance"
	"github.com/sarchlab/akita/v5/noc/networking/switching/endpoint"

	"github.com/sarchlab/akita/v5/noc/directconnection"
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
	}

	// The two endpoints are linked directly, so each one sends its flits to
	// the other's network port.
	netPorts := []messaging.Port{
		messaging.NewPort("EP1.NetworkPort", 4, 4),
		messaging.NewPort("EP2.NetworkPort", 4, 4),
	}

	for i, name := range []string{"EP1", "EP2"} {
		epSpec := endpoint.Definition.DefaultSpec
		epSpec.Freq = freq
		epSpec.FlitByteSize = 8
		epSpec.DefaultSwitchDst = netPorts[1-i].AsRemote()

		endpoint.Definition.Builder().
			WithSimulation(s).
			WithSpec(epSpec).
			WithResources(endpoint.Resources{DevicePorts: agents[i].AgentPorts}).
			WithPorts(endpoint.Ports{NetworkPort: netPorts[i]}).
			Build(name)
	}

	conn := directconnection.MakeBuilder().
		WithSimulation(s).
		Build("Conn")

	conn.PlugIn(netPorts[0])
	conn.PlugIn(netPorts[1])

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[1])
}
