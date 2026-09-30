// Package badmiddleware has a Middlewares field that is not a middleware. The
// inspector must reject it: every Middlewares field implements
// modeling.Middleware.
package badmiddleware

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct{ N int }

type (
	State struct{}
	Ports struct{}
)

type Middlewares struct {
	Pipeline *pipeline
}

// pipeline has no Handle method.
type pipeline struct{ comp *Comp }

func (p *pipeline) Tick() bool { return false }

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Pipeline: &pipeline{comp: c}}
}
