package model

import (
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
)

// SendError marks a failure send or receive
type SendError struct{}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Connection is responsible for delivering messages to its destination.
type Connection interface {
	naming.Named
	hooking.Hookable

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(port Port)
	NotifySend()
}

// HookPosConnStartSend marks a connection accept to send a message.
var HookPosConnStartSend = &hooking.HookPos{Name: "Conn Start Send"}

// HookPosConnStartTrans marks a connection start to transmit a message.
var HookPosConnStartTrans = &hooking.HookPos{Name: "Conn Start Trans"}

// HookPosConnDoneTrans marks a connection complete transmitting a message.
var HookPosConnDoneTrans = &hooking.HookPos{Name: "Conn Done Trans"}

// HookPosConnDeliver marks a connection delivered a message.
var HookPosConnDeliver = &hooking.HookPos{Name: "Conn Deliver"}
