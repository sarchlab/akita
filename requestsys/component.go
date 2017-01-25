package requestsys

// A Sender can send requests to their destinations
type Sender interface {
	CanSend(req *Request) bool
	Send(req *Request) error
}

// A Receiver can receive requests
type Receiver interface {
	CanRecv(req *Request) bool
	Recv(req *Request) error
}

// A Named object is an object that has a name
type Named interface {
	Name() string
}

// A Component is a element that is being simulated in Yaotsu.
type Component interface {
	Named

	AddSocket(s *Socket)
	GetSocketByName(name string) *Socket

	CanProcess(req *Request) bool
	Process(req *Request) error
}

// BasicComponent provides some functions that other component can use
type BasicComponent struct {
	name    string
	sockets map[string]*Socket
}

// NewBasicComponent creates a new basic component
func NewBasicComponent(name string) *BasicComponent {
	return &BasicComponent{name, make(map[string]*Socket)}
}

// Name returns the name of the BasicComponent
func (c *BasicComponent) Name() string {
	return c.name
}

// GetSocketByName returns the socket object according the socket name
func (c *BasicComponent) GetSocketByName(name string) *Socket {
	return c.sockets[name]
}

// AddSocket registers with
func (c *BasicComponent) AddSocket(s *Socket) {
	c.sockets[s.Name] = s
}

// BindSocket associates a socket with a component
func BindSocket(c Component, s *Socket) {
	s.Component = c
	c.AddSocket(s)
}
