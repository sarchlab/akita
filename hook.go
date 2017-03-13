package core

import (
	"log"
	"reflect"
)

// Hookable defines an object that accept hooks
type Hookable interface {
	// Accept registers a hook
	Accept(hook Hook)

	// Invoke triggers all hooks that is hooked at a certain type and a certain
	// HookPos to execute their HookFunc.
	Invoke(item interface{}, pos HookPos)
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
	// For example, a component can receive a hook that hooks either to a event,
	// or a Request.
	// Type can be nil. A nil Type means that this hook hooks to anything.
	Type() reflect.Type

	// The Pos determines when the Hookfunc should be invoked.
	Pos() HookPos

	// Func determines what to do if hook is invoked.
	Func(item interface{}, domain Hookable)
}

// A BasicHookable provides some utility function for other type that implment
// the Hookable interface.
type BasicHookable struct {
	hooks []Hook
}

// NewBasicHookable creats a BasicHookable object
func NewBasicHookable() *BasicHookable {
	h := new(BasicHookable)
	h.hooks = make([]Hook, 0)
	return h
}

// Accept register a hook
func (h *BasicHookable) Accept(hook Hook) {
	h.hooks = append(h.hooks, hook)
}

func (h *BasicHookable) tryInvoke(item interface{}, pos HookPos, hook Hook) {
	if hook.Pos() == Any || pos == hook.Pos() {
		hook.Func(item, h)
		return
	}
}

// Invoke trigers the register hooks
func (h *BasicHookable) Invoke(item interface{}, pos HookPos) {
	for _, hook := range h.hooks {
		log.Println(reflect.TypeOf(item), hook.Type())
		if hook.Type() == nil {
			h.tryInvoke(item, pos, hook)
		} else if hook.Type().Kind() == reflect.Ptr &&
			hook.Type().Elem().Kind() == reflect.Interface &&
			reflect.TypeOf(item).Implements(hook.Type().Elem()) {
			h.tryInvoke(item, pos, hook)
		} else if hook.Type() == reflect.TypeOf(item) {
			h.tryInvoke(item, pos, hook)
		}
	}
}
