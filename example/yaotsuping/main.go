package main

import (
	"fmt"
	"math/rand"

	"gitlab.com/yaotsu/core"
)

func main() {
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
	for i := 0; i < 10000000; i++ {
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
