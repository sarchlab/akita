// Package nomiddlewares leaves NewMiddlewares out of its Definition. The
// inspector must reject it: a component without middlewares cannot be built.
package nomiddlewares

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type Spec struct{ N int }

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{N: 1},
}
