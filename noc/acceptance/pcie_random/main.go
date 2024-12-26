package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/noc/acceptance"
	"github.com/sarchlab/akita/v4/noc/networking/pcie"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
	"github.com/tebeka/atexit"
)

var numDevicePerSwitch = 8
var numPortPerDevice = 9

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := timing.NewSerialEngine()
	// engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))
	sim := simulation.NewSimulation()
	sim.RegisterEngine(engine)

	t := acceptance.NewTest()

	createNetwork(sim, t)
	t.GenerateMsgs(10000)

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.Now())
	atexit.Exit(0)
}

func createNetwork(sim simulation.Simulation, test *acceptance.Test) {
	monitor := monitoring.NewMonitor()
	monitor.RegisterEngine(sim.GetEngine())
	monitor.StartServer()

	freq := 1.0 * timing.GHz

	var agents []*acceptance.Agent

	for i := 0; i < numDevicePerSwitch*2+1; i++ {
		agent := acceptance.NewAgent(
			sim, freq, fmt.Sprintf("Agent%d", i), numPortPerDevice, test)
		agent.TickLater()
		agents = append(agents, agent)
		test.RegisterAgent(agent)
		monitor.RegisterComponent(agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithSimulation(sim).
		WithFrequency(freq).
		WithMonitor(monitor).
		WithVersion(4, 16)

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(agents[0].AgentPorts)
	switch1ID := pcieConnector.AddSwitch(rootComplexID)

	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch1ID, agents[i+1].AgentPorts)
	}

	switch2ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch2ID,
			agents[i+1+numDevicePerSwitch].AgentPorts)
	}

	pcieConnector.EstablishRoute()
}
