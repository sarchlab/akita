package sim

import "github.com/sarchlab/akita/v5/sim/messaging"

// Entity is the abstract base interface for every registered runtime object.
// Components, ports, connections, and resources all satisfy it. It is the
// common vocabulary the global state manager uses to track entities and resolve
// them by name; the concrete kinds extend it with their own capabilities.
type Entity interface {
	Name() string
}

// Component is the minimal component contract the simulation runtime needs.
// Concrete messaging components satisfy this without the simulation package
// depending on messaging.
type Component interface {
	Entity
}

// Port is the common messaging contract used by the runtime inventory.
type Port = messaging.Port

// Connection is the minimal connection contract the simulation runtime needs.
// Every messaging.Connection satisfies it.
type Connection interface {
	Entity
}

// Resource is a shared-state entity: non-timing program state — such as memory
// contents or page tables — that can be referenced by multiple components. The
// simulation owns resources; components hold references to them and resolve them
// by name through the global state manager rather than embedding the payload in
// their own state. Setup constructs and registers each resource once under a
// canonical name.
type Resource interface {
	Entity
}
