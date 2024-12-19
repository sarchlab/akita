package cellsplit

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim/timing"
)

func Example() {
	engine := timing.NewSerialEngine()
	handler := &SplitHandler{
		rand:   rand.New(rand.NewSource(1)),
		engine: engine,
	}

	engine.Schedule(&SplitEvent{
		time:    0,
		handler: handler,
	})

	engine.Run()

	fmt.Printf("Total number at time 100: %d\n", handler.total)
	// Output: Total number at time 100: 69
}
