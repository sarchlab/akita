package testsim

import "github.com/sarchlab/akita/v5/sim/timing"

// Clock is an engine whose time a test moves explicitly, for tests that hand
// ticks to a component directly. Components that sleep measure elapsed time, so
// such a test must step the clock between ticks.
type Clock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

// NewClock returns a Clock at time zero.
func NewClock() *Clock {
	return &Clock{Engine: timing.NewSerialEngine()}
}

// CurrentTime returns the time the test last stepped to.
func (c *Clock) CurrentTime() timing.VTimeInPicoSec {
	return c.now
}

// Step moves the clock to the next edge of freq.
func (c *Clock) Step(freq timing.Freq) {
	c.now = freq.NextTick(c.now)
}
