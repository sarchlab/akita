package hooking

// HookPos defines the enum of possible hooking positions.
type HookPos struct {
	Name string
}

// HookCtx is the context that holds all the information about the site that a
// hook is triggered.
type HookCtx struct {
	// Domain is the hookable object that is raising this hook.
	Domain Hookable

	// Pos identifies the lifecycle stage or location the hook is firing from.
	Pos *HookPos

	// Item carries the primary subject associated with the hook (event, task,
	// msg).
	Item any

	// Detail holds optional auxiliary data; hook sites may leave it nil.
	Detail any
}

// Hookable defines an object that accept Hooks.
type Hookable interface {
	// AcceptHook registers a hook.
	//
	// Hooks must be registered during single-threaded configuration, before the
	// hookable domain starts running. Once a hook is attached it is expected to
	// remain for the lifetime of the domain; implementations do not support
	// removal, so disable work inside the hook if it should stop reacting.
	AcceptHook(hook Hook)

	// NumHooks returns the number of hooks registered.
	NumHooks() int

	// Hooks returns all the hooks registered.
	Hooks() []Hook

	// InvokeHook triggers the registered Hooks.
	InvokeHook(ctx HookCtx)
}

// Hook is a short piece of program that can be invoked by a hookable object.
type Hook interface {
	// Func determines what to do if hook is invoked.
	Func(ctx HookCtx)
}

// A HookableBase provides some utility function for other type that implement
// the Hookable interface.
type HookableBase struct {
	hookList []Hook
}

// NewHookableBase creates a HookableBase object.
func NewHookableBase() *HookableBase {
	h := new(HookableBase)
	h.hookList = make([]Hook, 0)

	return h
}

// NumHooks returns the number of hooks registered.
func (h *HookableBase) NumHooks() int {
	return len(h.hookList)
}

// Hooks returns all the hooks registered.
func (h *HookableBase) Hooks() []Hook {
	return h.hookList
}

// AcceptHook register a hook.
//
// Hook registration is expected to happen during component setup while only a
// single goroutine interacts with the hook list. After the domain enters the
// simulation, concurrent mutation of the slice is undefined behaviour, so store
// any desired disable logic inside the hook implementation instead of removing
// it later.
func (h *HookableBase) AcceptHook(hook Hook) {
	h.mustNotHaveDuplicatedHook(hook)
	h.hookList = append(h.hookList, hook)
}

func (h *HookableBase) mustNotHaveDuplicatedHook(hook Hook) {
	for _, h := range h.hookList {
		if h == hook {
			panic("duplicated hook")
		}
	}
}

// InvokeHook triggers the register Hooks.
func (h *HookableBase) InvokeHook(ctx HookCtx) {
	for _, hook := range h.hookList {
		hook.Func(ctx)
	}
}

var _ Hookable = (*HookableBase)(nil)
