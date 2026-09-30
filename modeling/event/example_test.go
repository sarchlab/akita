package event_test

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// A delay line completes every request it receives Latency later: a Recv
// event schedules a doneEvent, and the doneEvent records the completion.
func Example() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	delay := Definition.Builder().
		WithSimulation(sim).
		WithSpec(Spec{Latency: 10}).
		WithPorts(Ports{In: messaging.NewPort(nil, 4, 4, "Delay.In")}).
		Build("Delay")

	delay.Ports.In.Deliver(req{messaging.MsgMeta{ID: 1}})

	if err := engine.Run(); err != nil {
		panic(err)
	}

	for _, d := range delay.State.Done {
		fmt.Printf("request %d done at %d ps\n", d.ID, d.At)
	}
	// Output:
	// request 1 done at 10 ps
}
