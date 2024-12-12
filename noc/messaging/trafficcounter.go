package messaging

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// A TrafficCounter counts number of bytes transferred over a connection
type TrafficCounter struct {
	TotalData uint64
}

// Func adds the delivered traffic to the counter
func (c *TrafficCounter) Func(ctx *hooking.HookCtx) {
	if ctx.Pos != sim.HookPosConnDeliver {
		return
	}

	req := ctx.Item.(modeling.Msg)
	c.TotalData += uint64(req.Meta().TrafficBytes)
}
