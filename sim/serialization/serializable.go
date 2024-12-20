package serialization

import "github.com/sarchlab/akita/v4/sim/naming"

// Serializable is an interface that can be serialized and deserialized.
type Serializable interface {
	naming.Named

	Serialize() (map[string]any, error)
	Deserialize(map[string]any) error
}
