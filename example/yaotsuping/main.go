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

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	engine := core.NewParallelEngine()
	connection := core.NewDirectConnection()

	numAgents := 100

	agents := make([]*PingComponent, 0)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("agent%d", i)
		agent := NewPingComponent(name, engine)
		core.PlugIn(agent, "Ping", connection)
		agents = append(agents, agent)
	}

	t := 0.0
	for i := 0; i < 1000000; i++ {
		evt := NewPingSendEvent()
		evt.SetTime(core.VTimeInSec(t))

		from := rand.Uint32() % uint32(numAgents)
		to := rand.Uint32() % uint32(numAgents)

		evt.SetHandler(agents[from])
		evt.From = agents[from]
		evt.To = agents[to]

		engine.Schedule(evt)
		if i%numAgents == 0 {
			t += 0.2
		}
	}

	engine.Run()
}
