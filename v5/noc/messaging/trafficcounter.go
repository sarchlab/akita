package messaging

import "github.com/sarchlab/akita/v5/sim"

// A TrafficCounter counts number of bytes transferred over a connection
type TrafficCounter struct {
	TotalData uint64
}

// Func adds the delivered traffic to the counter
func (c *TrafficCounter) Func(ctx *sim.HookCtx) {
	if ctx.Pos != sim.HookPosConnDeliver {
		return
	}

	req := ctx.Item.(*sim.GenericMsg)
	c.TotalData += uint64(req.TrafficBytes)
}
