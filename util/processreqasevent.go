package util

import "gitlab.com/yaotsu/core"

// ProcessReqAsEvent reschedules the request as an event at next clock cycle
// determined by frequency.
func ProcessReqAsEvent(request core.Req, engine core.Engine, freq Freq) {
	request.SetRecvTime(freq.NextTick(request.RecvTime()))
	engine.Schedule(request)
}
