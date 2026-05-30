package simulation

// State is a reference to an entity's serializable runtime state. It is a plain
// data object — no behavior is required — and by default it is serialized with
// JSON.
//
// GetStateByName returns a State by reference, so a caller can read and mutate
// live state through the backdoor. State is an alias for any: callers
// type-assert the result to the concrete state type. That friction is
// intentional — it flags that you are reaching past the normal interfaces into
// another entity's internals.
type State = any

// StateHolder is implemented by an entity whose serializable state lives in a
// distinct sub-object — for example, a modeling.Component keeps its runtime
// data in a State field separate from the component value itself. StateRef
// returns a live reference to that state, so the reference must stay valid for
// the entity's lifetime: return a pointer to a live field, not a copy.
//
// Entities whose own value is already their state — such as a shared Resource,
// the engine, or the ID-generator handle — need not implement StateHolder;
// GetStateByName returns the entity itself in that case.
type StateHolder interface {
	StateRef() State
}

// stateOf returns the State an entity exposes to the global state manager: the
// StateHolder's StateRef when implemented, otherwise the entity value itself.
func stateOf(object any) State {
	if holder, ok := object.(StateHolder); ok {
		return holder.StateRef()
	}

	return object
}
