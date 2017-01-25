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
func (c *ReqSink) Send(req *Request) {
}

// PlugInto connects the socket with the connection
func (c *ReqSink) linkSocket(s *Socket) error {
	return nil
}

// Disconnect remove the association between the conneciton and the socket
func (c *ReqSink) unlinkSocket(s *Socket) error {
	return nil
}
