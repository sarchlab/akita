package naming

// Named describes an object that has a name.
type Named interface {
	// Name returns the name of the object.
	Name() string
}
