package core

// ProcessReqAsEvent reschedules the request as an event at next clock cycle
// determined by frequency.
func ProcessReqAsEvent(request Req, engine Engine, freq Freq) {
	request.SetEventTime(freq.ThisTick(request.RecvTime()))
	engine.Schedule(request)
}
