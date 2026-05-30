package simulation

// Resource represents non-timing program state that can be referenced by
// multiple components, such as memory contents or page tables. It is registered
// with the simulation as shared state and is reachable by name through the
// global state manager, so a component can resolve it with GetStateByName
// instead of embedding the payload in its own state.
type Resource interface {
	Name() string
	Kind() string
	Identity() string
}

// ResourceOwner is implemented by components that reference resources that
// should be registered with the simulation when the component is registered.
type ResourceOwner interface {
	Resources() []Resource
}
