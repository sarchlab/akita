package akita

// HookPos defines the enum of possible hooking positions
type HookPos struct {
	Name string
}

// HookCtx is the context that holds all the information about the site that a
// hook is triggered
type HookCtx struct {
	Domain Hookable
	Now    VTimeInSec
	Pos    *HookPos
	Item   interface{}
	Detail interface{}
}

// Hookable defines an object that accept Hooks
type Hookable interface {
	// AcceptHook registers a hook
	AcceptHook(hook Hook)
}

// HookPosBeforeEvent is a hook position that triggers before handling an event
var HookPosBeforeEvent = &HookPos{Name: "BeforeEvent"}

// HookPosAfterEvent is a hook position that triggers after handling an event
var HookPosAfterEvent = &HookPos{Name: "AfterEvent"}

// Hook is a short piece of program that can be invoked by a hookable object.
type Hook interface {
	// Func determines what to do if hook is invoked.
	Func(ctx *HookCtx)
}

// A HookableBase provides some utility function for other type that implement
// the Hookable interface.
type HookableBase struct {
	hooks []Hook
}

// NewHookableBase creates a HookableBase object
func NewHookableBase() *HookableBase {
	h := new(HookableBase)
	h.hooks = make([]Hook, 0)
	return h
}

// AcceptHook register a hook
func (h *HookableBase) AcceptHook(hook Hook) {
	h.hooks = append(h.hooks, hook)
}

// InvokeHook triggers the register Hooks
func (h *HookableBase) InvokeHook(ctx *HookCtx) {
	for _, hook := range h.hooks {
		hook.Func(ctx)
	}
}
