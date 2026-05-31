package simulation

// Resource is a shared-state entity: non-timing program state — such as memory
// contents or page tables — that can be referenced by multiple components. The
// simulation owns resources; components hold references to them and resolve them
// by name through the global state manager rather than embedding the payload in
// their own state. Setup constructs and registers each resource once under a
// canonical name.
type Resource interface {
	Entity
}
