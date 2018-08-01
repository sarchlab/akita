package core

import (
	"log"
	"math"
)

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
	if f == 0 {
		log.Panic("frequency cannot be 0")
	}
	return VTimeInSec(1.0 / f)
}

// ThisTick returns the current tick time
//
//
//                Input
//                (          ]
//     |----------|----------|----------|----->
//                           |
//                           Output
func (f Freq) ThisTick(now VTimeInSec) VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Panic("invalid time")
	}
	period := f.Period()
	count := math.Ceil(math.Round(float64(now/(period/10))) / 10)
	res := VTimeInSec(count) * period
	return res
}

// NextTick returns the next tick time.
//
//                Input
//                [          )
//     |----------|----------|----------|----->
//                           |
//                           Output
func (f Freq) NextTick(now VTimeInSec) VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Panic("invalid time")
	}
	period := f.Period()
	count := math.Floor(math.Round(float64((now+period)/(period/10))) / 10)
	return VTimeInSec(count+1) * period
}

// NCyclesLater returns the time after N cycles
//
// This function will always return a time of an integer number of cycles
func (f Freq) NCyclesLater(n int, now VTimeInSec) VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Panic("invalid time")
	}
	return f.ThisTick(now + VTimeInSec(n)*f.Period())
}

// NoEarlierThan returns the tick time that is at or right after the given time
func (f Freq) NoEarlierThan(t VTimeInSec) VTimeInSec {
	if math.IsNaN(float64(t)) {
		log.Panic("invalid time")
	}
	count := t / f.Period()
	return VTimeInSec(math.Ceil(float64(count))) * f.Period()
}

// HalfTick returns the time in middle of two ticks
//
//                Input
//                (          ]
//     |----------|----------|----------|----->
//                                |
//                                Output
//
func (f Freq) HalfTick(t VTimeInSec) VTimeInSec {
	return f.ThisTick(t) + f.Period()/2
}
