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
	tt sim.TimeTeller,
	perfLogger PerfLogger,
	period sim.VTimeInSec,
) *PortAnalyzer {
	h := &PortAnalyzer{
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
	return sim.VTimeInSec(math.Floor(float64(t / h.period)))
}

func (h *PortAnalyzer) periodEndTime(t sim.VTimeInSec) sim.VTimeInSec {
	return h.periodStartTime(t) + h.period
}

// func (h *PortAnalyzer) messageCount(
// 	floorEndTime sim.VTimeInSec,
// ) {
// 	_, has := h.messageCounter[floorEndTime]
// 	if has == false {
// 		h.messageCounter[floorEndTime] = 1
// 	}
// 	h.messageCounter[floorEndTime] = h.messageCounter[floorEndTime] + 1
// }

// func (h *PortAnalyzer) trafficBytesCount(
// 	msg sim.Msg,
// 	floorEndTime sim.VTimeInSec,
// ) {
// 	_, has := h.trafficBytesCounter[floorEndTime]
// 	if has == false {
// 		h.trafficBytesCounter[floorEndTime] = int64(msg.Meta().TrafficBytes)
// 	}
// 	h.trafficBytesCounter[floorEndTime] =
// 		h.trafficBytesCounter[floorEndTime] + int64(msg.Meta().TrafficBytes)
// }

// func (h *PortAnalyzer) lastMsg(
// 	floorEndTime sim.VTimeInSec,
// 	ctx sim.HookCtx,
// ) {
// 	msg, _ := ctx.Item.(sim.Msg)
// 	for time, _ := range h.messageCounter {
// 		if time < floorEndTime {
// 			h.logger.Printf("%.10f, %s,%s, %d, %d\n",
// 				time,
// 				ctx.Domain.(sim.Port).Name(),
// 				ctx.Pos.Name,
// 				h.messageCounter[time],
// 				h.trafficBytesCounter[time])
// 			delete(h.messageCounter, time)
// 			delete(h.trafficBytesCounter, time)
// 		} else {
// 			h.logger.Printf("%.10f, %s,%s, %d, %d\n",
// 				msg.Meta().RecvTime,
// 				ctx.Domain.(sim.Port).Name(),
// 				ctx.Pos.Name,
// 				h.messageCounter[floorEndTime]-1,
// 				h.trafficBytesCounter[floorEndTime])
// 			delete(h.messageCounter, time)
// 			delete(h.trafficBytesCounter, time)
// 		}
// 	}
// }

// func (h *PortBandwidthLogger) oneInterval(
// 	floorEndTime sim.VTimeInSec,
// 	ctx sim.HookCtx,
// ) {
// 	_, has := h.messageCounter[h.startTime]
// 	if has != false {
// 		h.logger.Printf("%.10f, %s,%s, %d, %d\n",
// 			floorEndTime,
// 			ctx.Domain.(sim.Port).Name(),
// 			ctx.Pos.Name,
// 			h.messageCounter[h.startTime],
// 			h.trafficBytesCounter[h.startTime])

// 		delete(h.messageCounter, h.startTime)
// 		delete(h.trafficBytesCounter, h.startTime)

// 		h.startTime = sim.VTimeInSec(floorEndTime)
// 	} else {
// 		h.startTime = floorEndTime
// 	}
// }

// func (h *PortBandwidthLogger) moreThanTwoInterval(
// 	floorEndTime sim.VTimeInSec,
// 	ctx sim.HookCtx,
// ) {
// 	_, has := h.messageCounter[h.startTime]
// 	if has != false {

// 		h.logger.Printf("%.10f, %s,%s, %d, %d\n",
// 			h.startTime+h.interval,
// 			ctx.Domain.(sim.Port).Name(),
// 			ctx.Pos.Name,
// 			h.messageCounter[h.startTime],
// 			h.trafficBytesCounter[h.startTime])

// 		delete(h.messageCounter, h.startTime)
// 		delete(h.trafficBytesCounter, h.startTime)

// 		h.startTime = sim.VTimeInSec(floorEndTime)
// 	} else {
// 		h.startTime = floorEndTime
// 	}
// }
