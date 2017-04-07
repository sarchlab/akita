package main

import (
	"gitlab.com/yaotsu/core"
)

// A PingReq is the Ping message send from one node to another
type PingReq struct {
	*core.ReqBase

	StartTime core.VTimeInSec
	IsReply   bool
}

// NewPingReq creates a new PingReq
func NewPingReq() *PingReq {
	return &PingReq{core.NewBasicRequest(), 0, false}
}
