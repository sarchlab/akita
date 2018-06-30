package core

// A MockComponent is a very simple component that is designed only for
// simplify the unit tests.
type MockComponent struct {
	*ComponentBase

	ReceivedReqs  []Req
	ReceiveErrors []*SendError

	ToOutside *Port
}

// NewMockComponent returns the a MockComponent
func NewMockComponent(name string) *MockComponent {
	c := new(MockComponent)
	c.ComponentBase = NewComponentBase(name)

	c.ToOutside = NewPort(c)

	return c
}

func (c *MockComponent) NotifyRecv(now VTimeInSec, port *Port) {

}

// Handle function of a MockComponent cannot handle any event
func (c *MockComponent) Handle(evt Event) error {
	return nil
}
