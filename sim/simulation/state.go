package simulation

import "github.com/sarchlab/akita/v4/sim/serialization"

// A State is a serializable object that can be used to save and load the state
// of a simulation.
type State interface {
	serialization.Serializable
}

// A StateHolder object is an object that can have its state saved and loaded.
type StateHolder interface {
	Name() string
	State() State
	SetState(State)
}
