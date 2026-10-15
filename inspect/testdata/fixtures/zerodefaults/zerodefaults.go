// Package zerodefaults leaves DefaultSpec out, so every Spec field defaults to
// its zero value, and declares no ports.
package zerodefaults

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Spec struct {
	Freq    timing.Freq `json:"freq"`
	Count   int
	Label   string
	Enabled bool
	Ratio   float32
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{}
