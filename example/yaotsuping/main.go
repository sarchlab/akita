package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"

	"gitlab.com/yaotsu/core"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func profileCPU() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

}

func main() {
	// profileCPU()

	engine := core.NewParallelEngine()
	connection := core.NewDirectConnection()

	numAgents := 4

	agents := make([]*PingComponent, 0)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("agent%d", i)
		agent := NewPingComponent(name, engine)
		core.PlugIn(agent, "Ping", connection)
		agents = append(agents, agent)
	}

	for i := 0; i < 10000; i++ {
		evt := NewPingSendEvent()

		from := rand.Uint32() % uint32(numAgents)
		to := rand.Uint32() % uint32(numAgents)
		time := rand.Uint32() % 100

		evt.SetTime(core.VTimeInSec(time))
		evt.SetHandler(agents[from])
		evt.From = agents[from]
		evt.To = agents[to]

		engine.Schedule(evt)
	}

	engine.Run()
}
