package akita

// ProcessMsgAsEvent reschedules the message as an event at next clock cycle
// determined by frequency.
func ProcessMsgAsEvent(msg Msg, engine Engine, freq Freq) {
	recvTime := msg.Meta().RecvTime
	eventTime := freq.ThisTick(recvTime)
	if eventTime < recvTime {
		eventTime = recvTime
	}
	msg.Meta().EventTime = eventTime
	engine.Schedule(msg)
}
