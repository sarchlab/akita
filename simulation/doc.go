// Package simulation provides the top-level simulation runner for the Akita
// simulation framework.
//
// A Simulation wires together the engine, data recorder, visual tracer, and
// optional monitoring server. It also acts as a global state manager: every
// registered runtime object — component, port, connection, shared-state
// resource, and the engine and ID generator singletons — is recorded in one
// flat entity inventory with a globally unique name, and can be resolved to a
// live reference to its state with GetStateByName.
package simulation
