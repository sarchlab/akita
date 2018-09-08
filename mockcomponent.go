package akita

// A MockComponent is a very simple component that is designed only for
// simplify the unit tests.
type MockComponent struct {
	*ComponentBase

	ReceivedReqs []Req

	ToOutside *Port
}

func (c *MockComponent) NotifyRecv(now VTimeInSec, port *Port) {
	req := port.Retrieve(now)
	c.ReceivedReqs = append(c.ReceivedReqs, req)
}

func (c *MockComponent) NotifyPortFree(now VTimeInSec, port *Port) {
}

// Handle function of a MockComponent cannot handle any event
func (c *MockComponent) Handle(evt Event) error {
	return nil
}

// NewMockComponent returns the a MockComponent
func NewMockComponent(name string) *MockComponent {
	c := new(MockComponent)
	c.ComponentBase = NewComponentBase(name)

	c.ToOutside = NewPort(c)

	return c
}
