package core

// A Req is the message element being transferred between compoenents
type Req interface {
	Src() Component
	SetSrc(c Component)
	Dst() Component
	SetDst(c Component)

	SetSendTime(t VTimeInSec)
	SendTime() VTimeInSec

	SetRecvTime(t VTimeInSec)
	RecvTime() VTimeInSec
}

// ReqBase provides some basic setter and getter for all other requests
type ReqBase struct {
	src      Component
	dst      Component
	sendTime VTimeInSec
	recvTime VTimeInSec
}

// NewBasicRequest creates a new BasicRequest
func NewBasicRequest() *ReqBase {
	return &ReqBase{nil, nil, 0, 0}
}

// SetSrc set the component that send the request
func (r *ReqBase) SetSrc(src Component) {
	r.src = src
}

// Src return the source of the BasicRequest
func (r *ReqBase) Src() Component {
	return r.src
}

// SetDst sets where the request needs to be sent to
func (r *ReqBase) SetDst(dst Component) {
	r.dst = dst
}

// Dst return the source of the BasicRequest
func (r *ReqBase) Dst() Component {
	return r.dst
}

// SetSendTime set the send time of the event
//
// The SendTime property helps the connection and the receiver know what
// time it is.
func (r *ReqBase) SetSendTime(t VTimeInSec) {
	r.sendTime = t
}

// SendTime returns when the request is sent
func (r *ReqBase) SendTime() VTimeInSec {
	return r.sendTime
}

// RecvTime return the time when the request is received
func (r *ReqBase) RecvTime() VTimeInSec {
	return r.recvTime
}

// SetRecvTime set the receive time of the request
//
// This field helps the receiver to know what time it is.
func (r *ReqBase) SetRecvTime(t VTimeInSec) {
	r.recvTime = t
}

// SwapSrcAndDst swaps the request source and the request destination
//
// This function is useful when the fulfiller returns the request to the
// sender.
func (r *ReqBase) SwapSrcAndDst() {
	r.src, r.dst = r.dst, r.src
}
