package sim

import (
	"github.com/sarchlab/akita/v5/sim/hook"
)

// Re-exports for backward compatibility.
type HookPos = hook.HookPos
type HookCtx = hook.HookCtx
type Hookable = hook.Hookable
type Hook = hook.Hook
type HookableBase = hook.HookableBase

var HookPosBeforeEvent = hook.HookPosBeforeEvent
var HookPosAfterEvent = hook.HookPosAfterEvent
var NewHookableBase = hook.NewHookableBase
