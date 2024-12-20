package stateful

import (
	"github.com/sarchlab/akita/v4/sim/naming"
)

// A State is a collection of data that can be serialized and deserialized.
type State interface {
	naming.Named

	Serialize() (map[string]interface{}, error)
	Deserialize(map[string]interface{}) error
}

// A StateHolder is a component that has a state.
type StateHolder interface {
	naming.Named

	State() State
	SetState(State)
}
