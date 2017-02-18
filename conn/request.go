package conn

import "gitlab.com/yaotsu/core/event"

// A Request is the message element being transferred between compoenents
type Request interface {
	Source() Component
	Destination() Component

	SetSendTime(t event.VTimeInSec)
	SendTime() event.VTimeInSec

	SetRecvTime(t event.VTimeInSec)
	RecvTime() event.VTimeInSec
}

// BasicRequest provides some basic setter and getter for all other requests
type BasicRequest struct {
	Src   Component
	Dst   Component
	TSend event.VTimeInSec
	TRecv event.VTimeInSec
}

// NewBasicRequest creates a new BasicRequest
func NewBasicRequest() *BasicRequest {
	return &BasicRequest{nil, nil, 0, 0}
}

// Source return the source of the BasicRequest
func (r *BasicRequest) Source() Component {
	return r.Src
}

// Destination return the source of the BasicRequest
func (r *BasicRequest) Destination() Component {
	return r.Dst
}

// SetSendTime set the send time of the event
//
// The SendTime property helps the connection and the receiver know what
// time it is.
func (r *BasicRequest) SetSendTime(t event.VTimeInSec) {
	r.TSend = t
}

// SendTime returns when the request is sent
func (r *BasicRequest) SendTime() event.VTimeInSec {
	return r.TSend
}

// RecvTime return the time when the request is received
func (r *BasicRequest) RecvTime() event.VTimeInSec {
	return r.TRecv
}

// SetRecvTime set the receive time of the request
//
// This field helps the receiver to know what time it is.
func (r *BasicRequest) SetRecvTime(t event.VTimeInSec) {
	r.TRecv = t
}

// SwapSrcAndDst swaps the request source and the request destination
//
// This function is useful when the fulfiller returns the request to the
// sender.
func (r *BasicRequest) SwapSrcAndDst() {
	r.Src, r.Dst = r.Dst, r.Src
}
