package analysis

import (
	"math"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

// PortAnalyzer is a hook for the amount of traffic that passes through a Port.
type PortAnalyzer struct {
	sim.TimeTeller
	PerfLogger

	lastTime sim.VTimeInSec
	period   sim.VTimeInSec

	port           sim.Port
	outTrafficByte uint64
	outTrafficMsg  uint64
	inTrafficByte  uint64
	inTrafficMsg   uint64
}

func NewPortAnalyzer(
	port sim.Port,
	tt sim.TimeTeller,
	perfLogger PerfLogger,
	period sim.VTimeInSec,
) *PortAnalyzer {
	h := &PortAnalyzer{
		port:       port,
		TimeTeller: tt,
		PerfLogger: perfLogger,
		period:     period,
	}

	atexit.Register(func() {
		h.summarizePeriod()
	})

	return h
}

// Func writes the message information into the logger
func (h *PortAnalyzer) Func(ctx sim.HookCtx) {
	msg, ok := ctx.Item.(sim.Msg)
	if !ok {
		return
	}

	now := h.CurrentTime()
	lastPeriodEndTime := h.periodEndTime(h.lastTime)

	if now > lastPeriodEndTime {
		h.summarizePeriod()
	}

	h.lastTime = now
	if msg.Meta().Dst == h.port {
		h.inTrafficByte += uint64(msg.Meta().TrafficBytes)
		h.inTrafficMsg++
	} else {
		h.outTrafficByte += uint64(msg.Meta().TrafficBytes)
		h.outTrafficMsg++
	}
}

func (h *PortAnalyzer) summarizePeriod() {
	startTime := h.periodStartTime(h.lastTime)
	endTime := h.periodEndTime(h.lastTime)

	if h.inTrafficMsg > 0 {
		h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
			Start: startTime,
			End:   endTime,
			Where: h.port.Name(),
			What:  "IncomingByte",
			Value: float64(h.inTrafficByte),
			Unit:  "Byte",
		})

		h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
			Start: startTime,
			End:   endTime,
			Where: h.port.Name(),
			What:  "IncomingMsg",
			Value: float64(h.inTrafficMsg),
			Unit:  "Msg",
		})
	}

	if h.outTrafficMsg > 0 {
		h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
			Start: startTime,
			End:   endTime,
			Where: h.port.Name(),
			What:  "OutGoingByte",
			Value: float64(h.outTrafficByte),
			Unit:  "Byte",
		})

		h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
			Start: startTime,
			End:   endTime,
			Where: h.port.Name(),
			What:  "OutGoingMsg",
			Value: float64(h.outTrafficMsg),
			Unit:  "Msg",
		})
	}

	h.inTrafficByte = 0
	h.inTrafficMsg = 0
	h.outTrafficByte = 0
	h.outTrafficMsg = 0
}

func (h *PortAnalyzer) periodStartTime(t sim.VTimeInSec) sim.VTimeInSec {
	return sim.VTimeInSec(math.Floor(float64(t/h.period))) * h.period
}

func (h *PortAnalyzer) periodEndTime(t sim.VTimeInSec) sim.VTimeInSec {
	return h.periodStartTime(t) + h.period
}
