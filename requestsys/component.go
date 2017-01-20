package requestsys

// A Component is a element that is being simulated in Yaotsu.
type Component interface {
	Name() string
	CanProcess(req *Request) bool
	Process(req *Request) error
}

// BasicComponent provides some functions that other component can use
type BasicComponent struct {
	name string
}

// NewBasicComponent creates a new basic component
func NewBasicComponent(name string) *BasicComponent {
	return &BasicComponent{name}
}

// Name returns the name of the BasicComponent
func (c *BasicComponent) Name() string {
	return c.name
}
