package model

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// PortMsgLogger is a hook for logging messages as they go across a Port
type PortMsgLogger struct {
	logger *log.Logger
	timing.TimeTeller
}

// NewPortMsgLogger returns a new PortMsgLogger which will write into the logger
func NewPortMsgLogger(
	logger *log.Logger,
	timeTeller timing.TimeTeller,
) *PortMsgLogger {
	h := new(PortMsgLogger)
	h.logger = logger
	h.TimeTeller = timeTeller

	return h
}

// Func writes the message information into the logger
func (h *PortMsgLogger) Func(ctx hooking.HookCtx) {
	msg, ok := ctx.Item.(Msg)
	if !ok {
		return
	}

	h.logger.Printf("%.10f,%s,%s,%s,%s,%s,%s\n",
		h.CurrentTime(),
		ctx.Domain.(Port).Name(),
		ctx.Pos.Name,
		msg.Meta().Src,
		msg.Meta().Dst,
		reflect.TypeOf(msg), msg.Meta().ID)
}
