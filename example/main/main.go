package main

import "gitlab.com/yaotsu/core/example"
import "gitlab.com/yaotsu/core/event"
import "gitlab.com/yaotsu/core/conn"

func main() {
	engine := event.NewSerialEngine()

	comp1 := example.NewPingComponent("comp1", engine)
	comp2 := example.NewPingComponent("comp2", engine)

	connection := conn.NewDirectConnection()

	comp1.Connect("Ping", connection)
	connection.Attach(comp1)
	comp2.Connect("Ping", connection)
	connection.Attach(comp2)

	t := 0.0
	for i := 0; i < 100; i++ {
		evt := example.NewPingSendEvent()
		evt.HappenTime = event.VTimeInSec(t)
		evt.From = comp1
		evt.To = comp2
		evt.Domain = comp1

		engine.Schedule(evt)
		t += 0.2
	}

	engine.Run()
}
