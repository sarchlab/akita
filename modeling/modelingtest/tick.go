package modelingtest

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

// Tick hands a TickEvent at the current time to each middleware of c, in
// field order, as c does on every cycle, and reports whether any of them made
// progress. Unlike c.Handle, it does not schedule the next tick, so a test
// can step the component one cycle at a time.
func Tick[S, T, R, P, M any](c *ticking.Component[S, T, R, P, M]) bool {
	return modeling.Dispatch(
		modeling.OrderedMiddlewares(&c.Middlewares), TickEvent(c))
}

// TickEvent returns a TickEvent for c at the current time, for a test that
// hands it to a single middleware.
func TickEvent[S, T, R, P, M any](
	c *ticking.Component[S, T, R, P, M],
) ticking.TickEvent {
	return ticking.MakeTickEvent(c.NewID(), c.Name(), c.CurrentTime())
}
