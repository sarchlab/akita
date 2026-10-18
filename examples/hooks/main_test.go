package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestMessageHookFiltersPositionsBeforeAssertingItem(t *testing.T) {
	hook := &msgHook{}
	require.NotPanics(t, func() {
		hook.Func(hooking.HookCtx{Pos: timing.HookPosBeforeEvent, Item: "not a message"})
	})
	for _, pos := range []*hooking.HookPos{messaging.HookPosPortMsgSend, messaging.HookPosPortMsgRecvd} {
		require.Panics(t, func() {
			hook.Func(hooking.HookCtx{Pos: pos, Item: "not a message"})
		})
		require.NotPanics(t, func() {
			hook.Func(hooking.HookCtx{Pos: pos, Item: messaging.Msg{}})
		})
	}
}
