package akita

import (
	"log"
	"reflect"
)

// PortMsgLogger is a hook for logging messages as they go across a Port
type PortMsgLogger struct {
	LogHookBase
}

// NewPortMsgLogger returns a new PortMsgLogger which will write into the logger
func NewPortMsgLogger(logger *log.Logger) *PortMsgLogger {
	h := new(PortMsgLogger)
	h.Logger = logger
	return h
}

// Func writes the message information into the logger
func (h *PortMsgLogger) Func(ctx HookCtx) {
	msg, ok := ctx.Item.(Msg)
	if !ok {
		return
	}
	if ctx.Pos == HookPosPortMsgRecvd {
		h.Logger.Printf("%.10f,", ctx.Now)
	} else if ctx.Pos == HookPosPortMsgRetrieve {
		h.Logger.Printf("%.10f,%s,%s\n", ctx.Now, reflect.TypeOf(msg), msg.Meta().ID)
	}
}
