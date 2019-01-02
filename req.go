package akita

import (
	"github.com/rs/xid"
)

// A Req is the message element being transferred between components
type Req interface {
	Src() Port
	SetSrc(c Port)
	Dst() Port
	SetDst(c Port)

	SetSendTime(t VTimeInSec)
	SendTime() VTimeInSec

	SetRecvTime(t VTimeInSec)
	RecvTime() VTimeInSec

	SetEventTime(t VTimeInSec)
	GetID() string

	SetByteSize(byteSize int)
	ByteSize() int

	// All requests are simply events that can be scheduled to the receiver
	Event
}

// ReqBase provides some basic setter and getter for all other requests
type ReqBase struct {
	ID        string
	src       Port
	dst       Port
	sendTime  VTimeInSec
	recvTime  VTimeInSec
	eventTime VTimeInSec
	byteSize  int
}

// NewReqBase creates a new BasicRequest
func NewReqBase() *ReqBase {
	r := new(ReqBase)
	r.ID = xid.New().String()
	return r
}

// SetSrc set the component that send the request
func (r *ReqBase) SetSrc(src Port) {
	r.src = src
}

// Src return the source of the BasicRequest
func (r *ReqBase) Src() Port {
	return r.src
}

// SetDst sets where the request needs to be sent to
func (r *ReqBase) SetDst(dst Port) {
	r.dst = dst
}

// Dst return the source of the BasicRequest
func (r *ReqBase) Dst() Port {
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

func (r *ReqBase) SetEventTime(t VTimeInSec) {
	r.eventTime = t
}

// Time returns the recv time of a request
func (r *ReqBase) Time() VTimeInSec {
	return r.eventTime
}

// Handler returns the receiver of the request
func (r *ReqBase) Handler() Handler {
	return r.dst.Component()
}

// GetID returns the ID of the request
func (r *ReqBase) GetID() string {
	return r.ID
}

func (r *ReqBase) SetByteSize(byteSize int) {
	r.byteSize = byteSize
}

func (r *ReqBase) ByteSize() int {
	return r.byteSize
}

// SwapSrcAndDst swaps the request source and the request destination
//
// This function is useful when the fulfiller returns the request to the
// sender.
func (r *ReqBase) SwapSrcAndDst() {
	r.src, r.dst = r.dst, r.src
}
