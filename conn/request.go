package conn

// A Request is the message element being transferred between compoenents
type Request interface {
	SetSource(c Component)
	Source() Component

	SetDestination(c Component)
	Destination() Component
}

// A BasicRequest provides the basic utilities that all requests may need.
// So all type of requests may want to embed a BasicRequest.
type BasicRequest struct {
	from Component
	to   Component
}

// NewBasicRequest creates a BasicRequest object
func NewBasicRequest() *BasicRequest {
	return &BasicRequest{nil, nil}
}

// Source returns the component that initiate the request
func (c *BasicRequest) Source() Component {
	return c.from
}

// SetSource sets the initiator of the request
func (c *BasicRequest) SetSource(comp Component) {
	c.from = comp
}

// Destination sets where the request need to be sent to. The destination
// is usually the Component that fulfill the request
func (c *BasicRequest) Destination() Component {
	return c.to
}

// SetDestination sets the component that the request need to be sent to
func (c *BasicRequest) SetDestination(comp Component) {
	c.to = comp
}
