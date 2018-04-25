package util

import (
	"log"
	"math"

	"gitlab.com/yaotsu/core"
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
func (f Freq) Period() core.VTimeInSec {
	if f == 0 {
		log.Fatal("frequency cannot be 0")
	}
	return core.VTimeInSec(1.0 / f)
}

// ThisTick returns the current tick time
//
//
//                Input
//                (          ]
//     |----------|----------|----------|----->
//                           |
//                           Output
func (f Freq) ThisTick(now core.VTimeInSec) core.VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Fatal("invalid time")
	}
	period := f.Period()
	count := math.Ceil(float64((now - period*1e-6) / period))
	return core.VTimeInSec(count) * period
}

// NextTick returns the next tick time.
//
//                Input
//                [          )
//     |----------|----------|----------|----->
//                           |
//                           Output
func (f Freq) NextTick(now core.VTimeInSec) core.VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Fatal("invalid time")
	}
	period := f.Period()
	count := math.Floor(float64((now + period*0.2) / period))
	return core.VTimeInSec(count+1) * period
}

// NCyclesLater returns the time after N cycles
//
// This function will always return a time of an integer number of cycles
func (f Freq) NCyclesLater(n int, now core.VTimeInSec) core.VTimeInSec {
	if math.IsNaN(float64(now)) {
		log.Fatal("invalid time")
	}
	return f.ThisTick(now + core.VTimeInSec(n)*f.Period())
}

// NoEarlierThan returns the tick time that is at or right after the given time
func (f Freq) NoEarlierThan(t core.VTimeInSec) core.VTimeInSec {
	if math.IsNaN(float64(t)) {
		log.Fatal("invalid time")
	}
	count := t / f.Period()
	return core.VTimeInSec(math.Ceil(float64(count))) * f.Period()
}

// HalfTick returns the time in middle of two ticks
//
//                Input
//                (          ]
//     |----------|----------|----------|----->
//                                |
//                                Output
//
func (f Freq) HalfTick(t core.VTimeInSec) core.VTimeInSec {
	return f.ThisTick(t) + f.Period()/2
}
