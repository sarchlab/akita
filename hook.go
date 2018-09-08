package akita

import (
	"reflect"
)

// Hookable defines an object that accept Hooks
type Hookable interface {
	// AcceptHook registers a hook
	AcceptHook(hook Hook)

	// InvokeHook triggers all Hooks that is hooked at a certain type and a
	// certain HookPos to execute their HookFunc.
	InvokeHook(item interface{}, domain Hookable, pos HookPos, info interface{})
}

// HookPos defines the enum of possible hooking positions
type HookPos int

// Enumeration of possible hook poses
const (
	Any HookPos = iota
	BeforeEvent
	AfterEvent
	OnRecvReq
	OnFulfillReq
	StatusChange
)

// Hook is a short piece of program that can be invoked by a hookable object.
type Hook interface {

	// Determines what type of item that the hook applies to.
	// For example, a component can receive a hook that Hooks either to a event,
	// or a Request.
	// Type can be nil. A nil Type means that this hook Hooks to anything.
	Type() reflect.Type

	// The Pos determines when the Hookfunc should be invoked.
	Pos() HookPos

	// Func determines what to do if hook is invoked. The item is the particular
	// thing that why the hook is invoked. The domain is the hookable component
	// that the hook is hooking to. The component can also pass some additional
	// information to the hook
	Func(item interface{}, domain Hookable, info interface{})
}

// A HookableBase provides some utility function for other type that implement
// the Hookable interface.
type HookableBase struct {
	Hooks []Hook
}

// NewHookableBase creates a HookableBase object
func NewHookableBase() *HookableBase {
	h := new(HookableBase)
	h.Hooks = make([]Hook, 0)
	return h
}

// AcceptHook register a hook
func (h *HookableBase) AcceptHook(hook Hook) {
	h.Hooks = append(h.Hooks, hook)
}

func (h *HookableBase) tryInvoke(
	item interface{},
	domain Hookable,
	pos HookPos,
	hook Hook,
	info interface{},
) {
	if hook.Pos() == Any || pos == hook.Pos() {
		hook.Func(item, domain, info)
		return
	}
}

// InvokeHook triggers the register Hooks
func (h *HookableBase) InvokeHook(
	item interface{},
	domain Hookable,
	pos HookPos,
	info interface{},
) {
	for _, hook := range h.Hooks {
		if hook.Type() == nil {
			h.tryInvoke(item, domain, pos, hook, info)
		} else if hook.Type().Kind() == reflect.Ptr &&
			hook.Type().Elem().Kind() == reflect.Interface &&
			reflect.TypeOf(item).Implements(hook.Type().Elem()) {
			h.tryInvoke(item, domain, pos, hook, info)
		} else if hook.Type() == reflect.TypeOf(item) {
			h.tryInvoke(item, domain, pos, hook, info)
		}
	}
}
