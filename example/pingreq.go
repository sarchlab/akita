package example

import (
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// A PingReq is the Ping message send from one node to another
type PingReq struct {
	*conn.BasicRequest

	StartTime event.VTimeInSec
	IsReply   bool
}

// NewPingReq creates a new PingReq
func NewPingReq() *PingReq {
	return &PingReq{conn.NewBasicRequest(), 0, false}
}
