package analysis

import (
	"fmt"
	"math"
	"strings"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

// PortAnalyzer is a hook for the amount of traffic that passes through a Port.
type PortAnalyzer struct {
	PerfLogger
	sim.TimeTeller

	usePeriod bool
	period    sim.VTimeInSec
	port      sim.Port

	lastTime       sim.VTimeInSec
	outTrafficByte uint64
	outTrafficMsg  uint64
	inTrafficByte  uint64
	inTrafficMsg   uint64
}

// Func writes the message information into the logger
func (h *PortAnalyzer) Func(ctx sim.HookCtx) {
	now := h.CurrentTime()
	msg, ok := ctx.Item.(sim.Msg)
	if !ok {
		return
	}

	if h.usePeriod {
		lastPeriodEndTime := h.periodEndTime(h.lastTime)

		if strings.Contains(msg.Meta().Src.Name(), "RDMA") {
			fmt.Printf("1\n")
		}

		if now > lastPeriodEndTime {
			if strings.Contains(msg.Meta().Src.Name(), "RDMA") {
				fmt.Printf("1\n")
			}
			h.summarize()
		}
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

func (h *PortAnalyzer) summarize() {
	now := h.CurrentTime()

	startTime := sim.VTimeInSec(0)
	endTime := now

	if h.usePeriod {
		startTime = h.periodStartTime(h.lastTime)
		endTime = h.periodEndTime(h.lastTime)

		if endTime > now {
			endTime = now
		}
	}

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

// PortAnalyzerBuilder can build a PortAnalyzer.
type PortAnalyzerBuilder struct {
	perfLogger PerfLogger
	timeTeller sim.TimeTeller
	usePeriod  bool
	period     sim.VTimeInSec
	port       sim.Port
}

// MakePortAnalyzerBuilder creates a PortAnalyzerBuilder.
func MakePortAnalyzerBuilder() PortAnalyzerBuilder {
	return PortAnalyzerBuilder{}
}

// WithPerfLogger sets the logger to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPerfLogger(l PerfLogger) PortAnalyzerBuilder {
	b.perfLogger = l
	return b
}

// WithTimeTeller sets the TimeTeller to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithTimeTeller(
	t sim.TimeTeller,
) PortAnalyzerBuilder {
	b.timeTeller = t
	return b
}

// WithPeriod sets the period to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPeriod(p sim.VTimeInSec) PortAnalyzerBuilder {
	b.usePeriod = true
	b.period = p
	return b
}

// WithPort sets the port to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPort(p sim.Port) PortAnalyzerBuilder {
	b.port = p
	return b
}

// Build creates a PortAnalyzer.
func (b PortAnalyzerBuilder) Build() *PortAnalyzer {
	if b.perfLogger == nil {
		panic("PortAnalyzer requires a PerfLogger")
	}

	if b.timeTeller == nil {
		panic("PortAnalyzer requires a TimeTeller")
	}

	if b.port == nil {
		panic("PortAnalyzer requires a Port")
	}

	a := &PortAnalyzer{
		PerfLogger: b.perfLogger,
		TimeTeller: b.timeTeller,
		usePeriod:  b.usePeriod,
		period:     b.period,
		port:       b.port,
	}

	atexit.Register(func() { a.summarize() })

	return a
}
