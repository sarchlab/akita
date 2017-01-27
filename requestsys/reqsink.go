package requestsys

// A ReqSink is a special type of connection. It simply ignore the requests
// to be sent over the connection
type ReqSink struct {
}

// CanSend of ReqSink always return true
func (c *ReqSink) CanSend(req *Request) bool {
	return true
}

// Send of ReqSink is intentionally left empty, since it simply discard the
// request
func (c *ReqSink) Send(req *Request) error {
	return nil
}

// Register of a ReqSink does not do anything, it does not care who connected
// to the system
func (c *ReqSink) Register(s Connectable) error {
	return nil
}

// Unregister of a ReqSink does not do anything
func (c *ReqSink) Unregister(s Connectable) error {
	return nil
}
