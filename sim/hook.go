package sim

import "github.com/sarchlab/akita/v4/instrumentation/hooking"

type (
	HookPos      = hooking.HookPos
	HookCtx      = hooking.HookCtx
	Hookable     = hooking.Hookable
	Hook         = hooking.Hook
	HookableBase = hooking.HookableBase
)

var (
	HookPosBeforeEvent = hooking.HookPosBeforeEvent
	HookPosAfterEvent  = hooking.HookPosAfterEvent
)

func NewHookableBase() *HookableBase {
	return hooking.NewHookableBase()
}
