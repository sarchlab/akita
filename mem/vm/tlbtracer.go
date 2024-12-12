package vm

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A TLBTracer write logs for what happened in a TLB
type TLBTracer struct {
	timeTeller timing.TimeTeller
	writer     io.Writer
}

// NewTLBTracer produce a new TLBTracer, injecting the dependency of a writer.
func NewTLBTracer(w io.Writer, timeTeller timing.TimeTeller) *TLBTracer {
	t := new(TLBTracer)
	t.writer = w
	t.timeTeller = timeTeller

	return t
}

// Func prints the tlb trace information.
func (t *TLBTracer) Func(ctx *hooking.HookCtx) {
	what, ok := ctx.Item.(string)
	if !ok {
		return
	}

	_, err := fmt.Fprintf(t.writer,
		"%.12f,%s,%s,{}\n",
		t.timeTeller.Now(),
		ctx.Domain.(modeling.Component).Name(),
		what)
	if err != nil {
		panic(err)
	}
}
