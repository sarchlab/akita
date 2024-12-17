package serialization

// Serializable is an interface that can be serialized and deserialized.
type Serializable interface {
	ID() string
	Serialize() (map[string]interface{}, error)
	Deserialize(map[string]interface{}) error
}
