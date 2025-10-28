package multiportsimplemem

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

type portArrivalHook struct {
	comp *Comp
}

func (h *portArrivalHook) Func(ctx sim.HookCtx) {
	if ctx.Pos != sim.HookPosPortMsgRecvd {
		return
	}

	req, ok := ctx.Item.(mem.AccessReq)
	if !ok {
		return
	}

	h.comp.recordArrival(req)
}
