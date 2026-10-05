package main

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

func TestMessageHookFiltersPositionsBeforeAssertingItem(t *testing.T) {
	g := NewWithT(t)
	hook := &msgHook{}
	g.Expect(func() {
		hook.Func(hooking.HookCtx{Pos: timing.HookPosBeforeEvent, Item: "not a message"})
	}).NotTo(Panic())
	for _, pos := range []*hooking.HookPos{messaging.HookPosPortMsgSend, messaging.HookPosPortMsgRecvd} {
		g.Expect(func() {
			hook.Func(hooking.HookCtx{Pos: pos, Item: "not a message"})
		}).To(Panic())
		g.Expect(func() {
			hook.Func(hooking.HookCtx{Pos: pos, Item: messaging.Msg{}})
		}).NotTo(Panic())
	}
}
