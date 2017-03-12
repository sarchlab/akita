package core

// A Request is the message element being transferred between compoenents
type Request interface {
	Source() Component
	SetSource(c Component)
	Destination() Component
	SetDestination(c Component)

	SetSendTime(t VTimeInSec)
	SendTime() VTimeInSec

	SetRecvTime(t VTimeInSec)
	RecvTime() VTimeInSec
}

// BasicRequest provides some basic setter and getter for all other requests
type BasicRequest struct {
	src      Component
	dst      Component
	sendTime VTimeInSec
	recvTime VTimeInSec
}

// NewBasicRequest creates a new BasicRequest
func NewBasicRequest() *BasicRequest {
	return &BasicRequest{nil, nil, 0, 0}
}

// SetSource set the component that send the request
func (r *BasicRequest) SetSource(src Component) {
	r.src = src
}

// Source return the source of the BasicRequest
func (r *BasicRequest) Source() Component {
	return r.src
}

// SetDestination sets where the request needs to be sent to
func (r *BasicRequest) SetDestination(dst Component) {
	r.dst = dst
}

// Destination return the source of the BasicRequest
func (r *BasicRequest) Destination() Component {
	return r.dst
}

// SetSendTime set the send time of the event
//
// The SendTime property helps the connection and the receiver know what
// time it is.
func (r *BasicRequest) SetSendTime(t VTimeInSec) {
	r.sendTime = t
}

// SendTime returns when the request is sent
func (r *BasicRequest) SendTime() VTimeInSec {
	return r.sendTime
}

// RecvTime return the time when the request is received
func (r *BasicRequest) RecvTime() VTimeInSec {
	return r.recvTime
}

// SetRecvTime set the receive time of the request
//
// This field helps the receiver to know what time it is.
func (r *BasicRequest) SetRecvTime(t VTimeInSec) {
	r.recvTime = t
}

// SwapSrcAndDst swaps the request source and the request destination
//
// This function is useful when the fulfiller returns the request to the
// sender.
func (r *BasicRequest) SwapSrcAndDst() {
	r.src, r.dst = r.dst, r.src
}
