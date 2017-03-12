package main

import (
	"gitlab.com/yaotsu/core"
)

func main() {
	engine := core.NewSerialEngine()

	comp1 := NewPingComponent("comp1", engine)
	comp2 := NewPingComponent("comp2", engine)

	connection := core.NewDirectConnection()

	core.PlugIn(comp1, "Ping", connection)
	core.PlugIn(comp2, "Ping", connection)

	t := 0.0
	for i := 0; i < 100; i++ {
		evt := NewPingSendEvent()
		evt.SetTime(core.VTimeInSec(t))
		evt.SetHandler(comp1)
		evt.From = comp1
		evt.To = comp2

		engine.Schedule(evt)
		t += 0.2
	}

	engine.Run()
}
