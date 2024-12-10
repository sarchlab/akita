package model

import "github.com/sarchlab/akita/v4/sim/timing"

// TickingComponent is a type of component that update states from cycle to
// cycle. A programmer would only need to program a tick function for a ticking
// component.
type TickingComponent struct {
	*ComponentBase
	*timing.TickScheduler

	ticker timing.Ticker
}

// NotifyPortFree triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyPortFree(
	_ Port,
) {
	c.TickLater()
}

// NotifyRecv triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyRecv(
	_ Port,
) {
	c.TickLater()
}

// Handle triggers the tick function of the TickingComponent
func (c *TickingComponent) Handle(e timing.Event) error {
	madeProgress := c.ticker.Tick()
	if madeProgress {
		c.TickLater()
	}

	return nil
}

// NewTickingComponent creates a new ticking component
func NewTickingComponent(
	name string,
	engine timing.Engine,
	freq timing.Freq,
	ticker timing.Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = timing.NewTickScheduler(tc, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker

	return tc
}

// NewSecondaryTickingComponent creates a new ticking component
func NewSecondaryTickingComponent(
	name string,
	engine timing.Engine,
	freq timing.Freq,
	ticker timing.Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = timing.NewSecondaryTickScheduler(tc, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker

	return tc
}
