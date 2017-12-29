package core

import (
	"reflect"
)

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

	// All requests are simply events that can be scheduled to the receiver
	Event
}

// ReqBase provides some basic setter and getter for all other requests
type ReqBase struct {
	src      Component
	dst      Component
	sendTime VTimeInSec
	recvTime VTimeInSec
}

// NewReqBase creates a new BasicRequest
func NewReqBase() *ReqBase {
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

// Time returns the recv time of a request
func (r *ReqBase) Time() VTimeInSec {
	return r.recvTime
}

// Handler returns the receiver of the request
func (r *ReqBase) Handler() Handler {
	return r.dst
}

// SwapSrcAndDst swaps the request source and the request destination
//
// This function is useful when the fulfiller returns the request to the
// sender.
func (r *ReqBase) SwapSrcAndDst() {
	r.src, r.dst = r.dst, r.src
}

// ReqEquivalent checks if two requests are equivalent to each other
func ReqEquivalent(r1 Req, r2 Req) bool {
	if r1 == r2 {
		return true
	}

	if reflect.TypeOf(r1) != reflect.TypeOf(r2) {
		// fmt.Printf("Type mismatch\n")
		return false
	}

	if r1.Src() != r2.Src() || r1.Dst() != r2.Dst() {
		// fmt.Printf("Src or dst mismatch\n")
		return false
	}

	reqType := reflect.TypeOf(r1)
	r1Value := reflect.ValueOf(r1)
	r2Value := reflect.ValueOf(r2)
	if reqType.Kind() == reflect.Ptr {
		reqType = reqType.Elem()
		r1Value = r1Value.Elem()
		r2Value = r2Value.Elem()
	}
	for i := 0; i < reqType.NumField(); i++ {
		field := reqType.Field(i)
		if field.Type == reflect.TypeOf((*ReqBase)(nil)).Elem() {
			continue
		}

		if !reflect.DeepEqual(r1Value.Field(i).Interface(), r2Value.Field(i).Interface()) {
			// fmt.Printf("Field %d, %s is not deeply equal\n",
			// 	i, r1Value.Field(i).String())
			return false
		}
	}

	return true
}
