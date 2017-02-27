package event

import "math"

// Freq defines the type of frequency
type Freq float64

// Defines the unit of frequency
const (
	Hz  Freq = 1
	KHz Freq = 1e3
	MHz Freq = 1e6
	GHz Freq = 1e9
)

// Period returns the time between two consecutive ticks
func (f Freq) Period() VTimeInSec {
	return VTimeInSec(1.0 / f)
}

// NextTick returns the next tick time.
//
// If currTime is not on a tick time, this function returns the time of
// next tick.
func (f Freq) NextTick(currTime VTimeInSec) VTimeInSec {
	period := f.Period()
	return VTimeInSec((math.Floor(float64(currTime/period)) + 1) * float64(period))
}

// NCyclesLater returns the time after N cycles
//
// This function will always return a time of an integer number of cycles
func (f Freq) NCyclesLater(n int, currTime VTimeInSec) VTimeInSec {
	return f.NextTick(currTime + VTimeInSec(n)*f.Period())
}
