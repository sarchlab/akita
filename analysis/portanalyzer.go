package analysis

import (
	"log"
	"math"

	"github.com/sarchlab/akita/v3/sim"
)

// PortBandwidthLogger is a hook for logging messages as they go across a Port
type PortBandwidthLogger struct {
	sim.LogHookBase
	sim.TimeTeller

	name      string
	startTime sim.VTimeInSec
	endTime   sim.VTimeInSec
	interval  sim.VTimeInSec

	messageCounter      map[sim.VTimeInSec]int64
	trafficBytesCounter map[sim.VTimeInSec]int64

	logger *log.Logger
}

// NewPortMsgLogger returns a new PortMsgLogger which will write into the logger
func NewPortBandwidthLogger(
	name string,
	interval sim.VTimeInSec,
	timeTeller sim.TimeTeller,
	logger *log.Logger,
) *PortBandwidthLogger {
	h := new(PortBandwidthLogger)

	h.name = name
	h.logger = logger
	h.TimeTeller = timeTeller
	h.interval = interval
	h.startTime = 0
	h.endTime = h.CurrentTime()
	h.messageCounter = make(map[sim.VTimeInSec]int64)
	h.trafficBytesCounter = make(map[sim.VTimeInSec]int64)

	return h
}

// Func writes the message information into the logger
func (h *PortBandwidthLogger) Func(ctx sim.HookCtx) {
	msg, ok := ctx.Item.(sim.Msg)
	if !ok {
		return
	}

	now := h.CurrentTime()

	floorEndTime := sim.VTimeInSec(math.Round(math.Floor(float64(now/h.interval))*float64(h.interval)*1e10) / 1e10)
	h.messageCount(floorEndTime)
	h.trafficBytesCount(msg, floorEndTime)

	diff := sim.VTimeInSec(math.Round(float64(floorEndTime-h.startTime)*1e10) / 1e10)

	msgID := msg.Meta().ID
	if msgID == "END" {
		h.lastMsg(floorEndTime, ctx)
	} else if diff >= 2*h.interval {
		h.moreThanTwoInterval(floorEndTime, ctx)
	} else if diff >= h.interval {
		h.oneInterval(floorEndTime, ctx)
	}
}

func (h *PortBandwidthLogger) messageCount(
	floorEndTime sim.VTimeInSec,
) {
	_, has := h.messageCounter[floorEndTime]
	if has == false {
		h.messageCounter[floorEndTime] = 1
	}
	h.messageCounter[floorEndTime] = h.messageCounter[floorEndTime] + 1
}

func (h *PortBandwidthLogger) trafficBytesCount(
	msg sim.Msg,
	floorEndTime sim.VTimeInSec,
) {
	_, has := h.trafficBytesCounter[floorEndTime]
	if has == false {
		h.trafficBytesCounter[floorEndTime] = int64(msg.Meta().TrafficBytes)
	}
	h.trafficBytesCounter[floorEndTime] =
		h.trafficBytesCounter[floorEndTime] + int64(msg.Meta().TrafficBytes)
}

func (h *PortBandwidthLogger) lastMsg(
	floorEndTime sim.VTimeInSec,
	ctx sim.HookCtx,
) {
	msg, _ := ctx.Item.(sim.Msg)
	for time, _ := range h.messageCounter {
		if time < floorEndTime {
			h.logger.Printf("%.10f, %s,%s, %d, %d\n",
				time,
				ctx.Domain.(sim.Port).Name(),
				ctx.Pos.Name,
				h.messageCounter[time],
				h.trafficBytesCounter[time])
			delete(h.messageCounter, time)
			delete(h.trafficBytesCounter, time)
		} else {
			h.logger.Printf("%.10f, %s,%s, %d, %d\n",
				msg.Meta().RecvTime,
				ctx.Domain.(sim.Port).Name(),
				ctx.Pos.Name,
				h.messageCounter[floorEndTime]-1,
				h.trafficBytesCounter[floorEndTime])
			delete(h.messageCounter, time)
			delete(h.trafficBytesCounter, time)
		}
	}
}

func (h *PortBandwidthLogger) oneInterval(
	floorEndTime sim.VTimeInSec,
	ctx sim.HookCtx,
) {
	_, has := h.messageCounter[h.startTime]
	if has != false {
		h.logger.Printf("%.10f, %s,%s, %d, %d\n",
			floorEndTime,
			ctx.Domain.(Port).Name(),
			ctx.Pos.Name,
			h.messageCounter[h.startTime],
			h.trafficBytesCounter[h.startTime])

		delete(h.messageCounter, h.startTime)
		delete(h.trafficBytesCounter, h.startTime)

		h.startTime = sim.VTimeInSec(floorEndTime)
	} else {
		h.startTime = floorEndTime
	}
}

func (h *PortBandwidthLogger) moreThanTwoInterval(
	floorEndTime sim.VTimeInSec,
	ctx sim.HookCtx,
) {
	_, has := h.messageCounter[h.startTime]
	if has != false {

		h.logger.Printf("%.10f, %s,%s, %d, %d\n",
			h.startTime+h.interval,
			ctx.Domain.(sim.Port).Name(),
			ctx.Pos.Name,
			h.messageCounter[h.startTime],
			h.trafficBytesCounter[h.startTime])

		delete(h.messageCounter, h.startTime)
		delete(h.trafficBytesCounter, h.startTime)

		h.startTime = sim.VTimeInSec(floorEndTime)
	} else {
		h.startTime = floorEndTime
	}
}
