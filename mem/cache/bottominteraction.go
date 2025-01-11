package cache

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem"
)

// bottomInteraction is a middleware that handles the interaction between the
// cache and the memory below it.
type bottomInteraction struct {
	*Comp
}

func (b *bottomInteraction) Tick() bool {
	madeProgress := false

	for range b.numReqPerCycle {
		madeProgress = b.topDown() || madeProgress
	}

	for range b.numReqPerCycle {
		madeProgress = b.bottomUp() || madeProgress
	}

	return madeProgress
}

func (b *bottomInteraction) topDown() bool {
	item := b.bottomInteractionBuf.Peek()
	if item == nil {
		return false
	}

	trans := item.(*transaction)
	reqToBottom := trans.reqToBottom

	err := b.bottomPort.Send(reqToBottom)
	if err != nil {
		return false
	}

	b.bottomInteractionBuf.Pop()

	return true
}

func (b *bottomInteraction) bottomUp() bool {
	item := b.bottomPort.PeekIncoming()
	if item == nil {
		return false
	}

	switch rsp := item.(type) {
	case mem.DataReadyRsp:
		return b.processDataReadyRsp(rsp)
	default:
		panic(fmt.Sprintf("unexpected type: %T", rsp))
	}
}

func (b *bottomInteraction) processDataReadyRsp(rsp mem.DataReadyRsp) bool {
	if !b.storageBottomUpBuf.CanPush() {
		return false
	}

	rspToID := rsp.RespondTo

	trans, found := b.findTransByReqToBottomID(rspToID)
	if !found {
		panic(fmt.Sprintf("transaction with reqID %s not found", rspToID))
	}

	trans.rspFromBottom = rsp
	b.storageBottomUpBuf.Push(trans)
	b.bottomPort.RetrieveIncoming()

	b.traceReqToBottomEnd(trans)

	return true
}
