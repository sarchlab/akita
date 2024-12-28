package cache

import (
	"reflect"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

func (c *Comp) traceReqStart(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskStart{
			ID:       modeling.ReqInTaskID(req.Meta().ID),
			ParentID: modeling.ReqOutTaskID(req.Meta().ID),
			Kind:     "req_in",
			What:     reflect.TypeOf(req).String(),
			Where:    c.Name(),
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) traceReqEnd(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskEnd,
		Item: hooking.TaskEnd{
			ID: modeling.ReqInTaskID(req.Meta().ID),
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) traceReqToBottomStart(trans *transaction) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskStart{
			ID:       modeling.ReqOutTaskID(trans.reqToBottom.Meta().ID),
			ParentID: modeling.ReqInTaskID(trans.req.Meta().ID),
			Kind:     "req_to_bottom",
			What:     reflect.TypeOf(trans.reqToBottom).String(),
			Where:    c.Name(),
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) traceReqToBottomEnd(trans *transaction) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskEnd,
		Item: hooking.TaskEnd{
			ID: modeling.ReqOutTaskID(trans.reqToBottom.Meta().ID),
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) tagMSHRHit(trans *transaction) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskTag{
			TaskID: modeling.ReqInTaskID(trans.req.Meta().ID),
			What:   "mshr_hit",
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) tagCacheHit(trans *transaction) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskTag{
			TaskID: modeling.ReqInTaskID(trans.req.Meta().ID),
			What:   "cache_hit",
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) tagCacheMiss(trans *transaction) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskTag{
			TaskID: modeling.ReqInTaskID(trans.req.Meta().ID),
			What:   "cache_miss",
		},
	}

	c.InvokeHook(ctx)
}
