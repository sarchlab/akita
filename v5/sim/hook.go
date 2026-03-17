package sim

import "github.com/sarchlab/akita/v5/hooking"

// HookPos defines the enum of possible hooking positions.
type HookPos = hooking.HookPos

// HookCtx is the context that holds all the information about the site that a
// hook is triggered.
type HookCtx = hooking.HookCtx

// Hookable defines an object that accept Hooks.
type Hookable = hooking.Hookable

// Hook is a short piece of program that can be invoked by a hookable object.
type Hook = hooking.Hook

// A HookableBase provides some utility function for other type that implement
// the Hookable interface.
type HookableBase = hooking.HookableBase

// NewHookableBase creates a HookableBase object.
var NewHookableBase = hooking.NewHookableBase

// HookPosBeforeEvent is a hook position that triggers before handling an event.
var HookPosBeforeEvent = &HookPos{Name: "BeforeEvent"}

// HookPosAfterEvent is a hook position that triggers after handling an event.
var HookPosAfterEvent = &HookPos{Name: "AfterEvent"}
