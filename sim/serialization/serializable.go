package serialization

// Serializable is an interface that can be serialized and deserialized.
type Serializable interface {
	ID() string
	Serialize() (map[string]any, error)
	Deserialize(map[string]any) error
}
