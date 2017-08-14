package main

import (
	"flag"
	"fmt"
	"math/rand"
	_ "net/http/pprof"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/engines"
)

var cpuprofile = flag.String("cpuprofile", "cpuprof.prof", "write cpu profile to file")

func main() {
	// flag.Parse()
	// if *cpuprofile != "" {
	// 	f, err := os.Create(*cpuprofile)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	pprof.StartCPUProfile(f)
	// 	defer pprof.StopCPUProfile()
	// }

	// runtime.SetBlockProfileRate(1)
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	engine := engines.NewSerialEngine()
	connection := connections.NewDirectConnection(engine)

	numAgents := 4

	agents := make([]*PingComponent, 0)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("agent%d", i)
		agent := NewPingComponent(name, engine)
		core.PlugIn(agent, "Ping", connection)
		agents = append(agents, agent)
	}

	for i := 0; i < 1000; i++ {

		from := rand.Uint32() % uint32(numAgents)
		to := rand.Uint32() % uint32(numAgents)
		time := rand.Uint32() % 100

		evt := NewPingSendEvent(core.VTimeInSec(time), agents[from])

		evt.From = agents[from]
		evt.To = agents[to]

		engine.Schedule(evt)
	}

	engine.Run()
}
